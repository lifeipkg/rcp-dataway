package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("内嵌默认配置应能加载: %v", err)
	}
	if cfg.Port != "8090" {
		t.Fatalf("端口默认 8090，得到 %s", cfg.Port)
	}
	if cfg.DatasourceName != "main" {
		t.Fatalf("数据源名默认 main，得到 %s", cfg.DatasourceName)
	}
	if cfg.DefaultMaxRows != 100 {
		t.Fatalf("默认行数上限 100，得到 %d", cfg.DefaultMaxRows)
	}
	if len(cfg.AllowedTables) != 4 {
		t.Fatalf("表白名单应为 4 张，得到 %v", cfg.AllowedTables)
	}
	// iam 的写入路径依赖这些模板存在；少一条就是某条业务写入在运行时 400。
	for _, id := range []string{
		"event.insert", "verdict.insert", "iam.user.insert", "iam.user.update",
		"iam.session.insert", "iam.session.delete", "iam.session.deleteByUser",
		"iam.session.cleanExpired",
	} {
		if _, ok := cfg.Templates[id]; !ok {
			t.Fatalf("模板 %s 缺失，已登记: %v", id, cfg.Templates)
		}
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("DATAWAY_DATASOURCE", "replica")
	t.Setenv("DATAWAY_DB_DSN", "postgres://u:p@db:5432/rc?sslmode=disable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Port != "9090" || cfg.DatasourceName != "replica" {
		t.Fatalf("端口/数据源名未被环境变量覆盖: %+v", cfg)
	}
	if cfg.DSN != "postgres://u:p@db:5432/rc?sslmode=disable" {
		t.Fatalf("DSN 未被覆盖: %s", cfg.DSN)
	}
}

func TestLoadComposesDSNFromParts(t *testing.T) {
	t.Setenv("DB_HOST", "postgres")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_NAME", "risk_control")
	t.Setenv("DB_USER", "risk_app")
	t.Setenv("DB_PASSWORD", "p@ss/w:rd")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	// 口令里的 @ / : 必须被转义，否则连接串会被解析错位
	if !strings.HasPrefix(cfg.DSN, "postgres://risk_app:") {
		t.Fatalf("DSN 前缀不符: %s", cfg.DSN)
	}
	if strings.Contains(cfg.DSN, "p@ss/w:rd") {
		t.Fatalf("口令未做 URL 转义: %s", cfg.DSN)
	}
	if !strings.Contains(cfg.DSN, "@postgres:5433/risk_control") {
		t.Fatalf("host/port/db 拼装不符: %s", cfg.DSN)
	}
	if !strings.Contains(cfg.DSN, "sslmode=disable") {
		t.Fatalf("sslmode 缺省应为 disable: %s", cfg.DSN)
	}
}

func TestLoadExplicitDSNWinsOverParts(t *testing.T) {
	t.Setenv("DATAWAY_DB_DSN", "postgres://explicit@db/rc")
	t.Setenv("DB_HOST", "ignored")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.DSN != "postgres://explicit@db/rc" {
		t.Fatalf("显式 DSN 应优先于分量拼装，得到 %s", cfg.DSN)
	}
}

func TestLoadRejectsMissingConfigFile(t *testing.T) {
	t.Setenv("DATAWAY_CONFIG", filepath.Join(t.TempDir(), "nope.yaml"))
	if _, err := Load(); err == nil {
		t.Fatal("DATAWAY_CONFIG 指向不存在的文件时应报错，不能静默回落到内嵌默认值")
	}
}

func TestLoadRejectsEmptyAllowlist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dataway.yaml")
	content := "port: 8090\ndatasource:\n  name: main\n  dsn: postgres://x\ntemplates:\n  a.b: \"DELETE FROM t WHERE id = ?\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	t.Setenv("DATAWAY_CONFIG", path)

	if _, err := Load(); err == nil {
		t.Fatal("空白名单应拒绝启动")
	}
}
