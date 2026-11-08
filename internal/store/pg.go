// Package store 是网关唯一持有数据库连接的地方：业务仓不持驱动（RL-005），
// 连接与凭据只出现在这里。
package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB 是网关需要的最小数据库面。接口存在的唯一理由是让 handler 的拒绝路径
// 能在没有真实 PostgreSQL 的情况下被断言（见 web/handler_test.go 的 fakeDB）。
type DB interface {
	Query(ctx context.Context, sql string, args []any, maxRows int) (Result, error)
	Exec(ctx context.Context, sql string, args []any) (int64, error)
	Ping(ctx context.Context) error
	Close()
}

// Result 是一次查询的出口形状：列名统一小写，行是按列名索引的字典。
type Result struct {
	Columns []string
	Rows    []map[string]any
}

// Pool 是 DB 的生产实现。
type Pool struct {
	pool *pgxpool.Pool
}

// Open 建连接池并做一次 Ping：连不上就启动失败，不带着坏连接对外服务。
func Open(ctx context.Context, dsn string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("连接串不可解析: %w", err)
	}
	// 网关是短查询面：连接数有上限，超时短，避免把数据库连接占满。
	cfg.MaxConns = 16
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("建立连接池失败: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("数据库不可达: %w", err)
	}
	return &Pool{pool: pool}, nil
}

func (p *Pool) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *Pool) Close() { p.pool.Close() }

// Query 执行一条已改写的 SELECT，最多读 maxRows 行。
//
// maxRows 在读取端截断而不是拼 LIMIT：拼 LIMIT 会改写调用方的语句，
// 让"网关只转发判断过的 SQL"这条边界变得含糊。
func (p *Pool) Query(ctx context.Context, sql string, args []any, maxRows int) (Result, error) {
	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	columns := make([]string, len(fields))
	// 列名统一小写：跨库大小写规则不一致，网关出口统一口径。
	for i, field := range fields {
		columns[i] = strings.ToLower(field.Name)
	}

	result := Result{Columns: columns, Rows: make([]map[string]any, 0, 16)}
	for rows.Next() {
		if maxRows > 0 && len(result.Rows) >= maxRows {
			break
		}
		values, err := rows.Values()
		if err != nil {
			return Result{}, err
		}
		row := make(map[string]any, len(columns))
		for i, value := range values {
			row[columns[i]] = normalize(value)
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (p *Pool) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	tag, err := p.pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// normalize 把 pgx 的扩展类型收成 encoding/json 能直接输出的形状。
//
// numeric 默认映射成 pgtype.Numeric，直接序列化会得到一个嵌套对象；调用方看到
// 的应该是数字。其余类型（time.Time / []byte / int32 / bool）保持原样，
// 与 Java 侧 getObject 的口径接近。
func normalize(value any) any {
	if numeric, ok := value.(pgtype.Numeric); ok {
		if numeric.NaN || numeric.InfinityModifier != pgtype.Finite {
			return nil
		}
		if asFloat, err := numeric.Float64Value(); err == nil && asFloat.Valid {
			return asFloat.Float64
		}
		return numeric.Int.String()
	}
	return value
}
