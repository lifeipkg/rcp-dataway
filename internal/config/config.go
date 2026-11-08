package config

import (
	_ "embed"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// 默认配置随二进制内嵌：部署侧不改清单也能起来，改白名单/模板不必重新打包。
//
//go:embed dataway.yaml
var embedded []byte

// file 是 YAML 的线上形状，字段名与 internal/config/dataway.yaml 一一对应。
type file struct {
	Port           int               `yaml:"port"`
	Datasource     datasource        `yaml:"datasource"`
	DefaultMaxRows int               `yaml:"default-max-rows"`
	AllowedTables  []string          `yaml:"allowed-tables"`
	Templates      map[string]string `yaml:"templates"`
}

type datasource struct {
	Name string `yaml:"name"`
	DSN  string `yaml:"dsn"`
}

// Config 是装配好的运行时配置；白名单与模板的量纲由 guard 再校验一遍。
type Config struct {
	Port           string
	DatasourceName string
	DSN            string
	DefaultMaxRows int
	AllowedTables  []string
	Templates      map[string]string
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Load 读取配置：内嵌默认值 → 可选外部文件 → 环境变量覆盖。
//
// DATAWAY_CONFIG 指向外部 YAML 时整份替换（不合并），避免"内嵌一半、外部一半"
// 这种没人能复算的最终状态。
func Load() (Config, error) {
	raw := embedded
	if path := os.Getenv("DATAWAY_CONFIG"); path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("读取 DATAWAY_CONFIG 失败: %w", err)
		}
		raw = content
	}

	var f file
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return Config{}, fmt.Errorf("配置不是合法 YAML: %w", err)
	}

	port := f.Port
	if port <= 0 {
		port = 8090
	}
	maxRows := f.DefaultMaxRows
	if maxRows <= 0 {
		maxRows = 100
	}
	name := f.Datasource.Name
	if name == "" {
		name = "main"
	}

	cfg := Config{
		Port:           env("SERVER_PORT", strconv.Itoa(port)),
		DatasourceName: env("DATAWAY_DATASOURCE", name),
		DSN:            resolveDSN(f.Datasource.DSN),
		DefaultMaxRows: maxRows,
		AllowedTables:  f.AllowedTables,
		Templates:      f.Templates,
	}

	if cfg.DSN == "" {
		return Config{}, fmt.Errorf("数据源未配置：给 DATAWAY_DB_DSN，或给 DB_HOST/DB_PORT/DB_NAME/DB_USER/DB_PASSWORD")
	}
	if len(cfg.AllowedTables) == 0 {
		return Config{}, fmt.Errorf("表白名单为空：没有白名单时网关会拒绝全部查询，不应启动")
	}
	if len(cfg.Templates) == 0 {
		return Config{}, fmt.Errorf("写入模板为空：模板是写入的唯一入口，空表意味着写入面失效")
	}
	return cfg, nil
}

// resolveDSN 决定连接串，优先级：显式 DSN > 分量拼装 > 文件默认。
//
// 分量拼装是为了让 K8s 能复用既有的 rcp-db-credential Secret（username / password
// 两个 key），而不必把整条 DSN 塞进 Secret——整条 DSN 会让"换口令"变成"改连接串"。
func resolveDSN(fallback string) string {
	if dsn := os.Getenv("DATAWAY_DB_DSN"); dsn != "" {
		return dsn
	}
	host := os.Getenv("DB_HOST")
	if host == "" {
		return fallback
	}
	port := env("DB_PORT", "5432")
	dbName := env("DB_NAME", "risk_control")
	user := env("DB_USER", "risk_app")
	pass := os.Getenv("DB_PASSWORD")

	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + dbName,
	}
	q := u.Query()
	q.Set("sslmode", env("DB_SSLMODE", "disable"))
	u.RawQuery = q.Encode()
	return u.String()
}
