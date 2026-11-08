package guard

import (
	"errors"
	"strings"
	"testing"
)

func newTestGuard() *Guard {
	return NewGuard([]string{"iam_users", "iam_sessions"})
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var rejected *Rejected
	if !errors.As(err, &rejected) {
		t.Fatalf("期望 Rejected 错误，得到 %v", err)
	}
	return rejected.Code
}

func TestValidateAllowsWhitelistedSelect(t *testing.T) {
	got, err := newTestGuard().Validate("  SELECT * FROM iam_sessions WHERE session_id = ? ;  ")
	if err != nil {
		t.Fatalf("白名单单表 SELECT 应放行: %v", err)
	}
	// 尾分号去掉、首尾空白去掉；占位符原样保留（改写是下一步）
	if want := "SELECT * FROM iam_sessions WHERE session_id = ?"; got != want {
		t.Fatalf("语句规范化不符\n得到: %s\n期望: %s", got, want)
	}
}

func TestValidateAllowsJoinOnWhitelistedTables(t *testing.T) {
	sql := "SELECT e.session_id FROM iam_users e JOIN iam_sessions v ON e.session_id = v.session_id"
	if _, err := newTestGuard().Validate(sql); err != nil {
		t.Fatalf("白名单表 join 应放行: %v", err)
	}
}

func TestValidateAllowsLowerCaseSelectAndQuotedTable(t *testing.T) {
	for _, sql := range []string{
		"select * from iam_users",
		`SELECT * FROM "iam_sessions"`,
		"SELECT * FROM public.iam_sessions",
		"SELECT (SELECT count(*) FROM iam_users) AS c FROM iam_sessions",
	} {
		if _, err := newTestGuard().Validate(sql); err != nil {
			t.Fatalf("%q 应放行，得到 %v", sql, err)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		code string
	}{
		{"空语句", "  ", CodeEmptySQL},
		{"多语句夹带 DDL", "SELECT 1 FROM iam_users; DROP TABLE iam_users", CodeMultiStatement},
		{"注释截断审计", "SELECT * FROM iam_users -- 注释", CodeCommentNotAllowed},
		{"块注释", "SELECT /* x */ * FROM iam_users", CodeCommentNotAllowed},
		{"非 SELECT", "UPDATE iam_sessions SET expires_at = 0", CodeOnlySelect},
		{"CTE 夹带写操作", "SELECT * FROM (DELETE FROM iam_users RETURNING session_id) t", CodeDMLNotAllowed},
		{"白名单外的表", "SELECT * FROM user_credentials", CodeTableNotAllowed},
		{"schema 前缀按最后一段判定", "SELECT * FROM public.sys_user", CodeTableNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newTestGuard().Validate(tc.sql)
			if code := codeOf(t, err); code != tc.code {
				t.Fatalf("%q 期望 %s，得到 %s", tc.sql, tc.code, code)
			}
		})
	}
}

func TestValidateRejectionMessageListsAllowedTables(t *testing.T) {
	_, err := newTestGuard().Validate("SELECT * FROM user_credentials")
	var rejected *Rejected
	if !errors.As(err, &rejected) {
		t.Fatalf("期望 Rejected，得到 %v", err)
	}
	if !strings.Contains(rejected.Message, "iam_sessions") {
		t.Fatalf("拒绝信息应给出允许范围，得到 %q", rejected.Message)
	}
}

func TestValidateReportsFirstHitOnly(t *testing.T) {
	// 同时命中多条规则时只报先判定的那条（注释先于 SELECT 前缀、DML 先于白名单）：
	// 调用方不需要猜优先级。
	cases := []struct {
		sql  string
		code string
	}{
		{"DELETE /* c */ FROM iam_users", CodeCommentNotAllowed},
		{"SELECT 1 FROM iam_users; DROP TABLE iam_users", CodeMultiStatement},
		{"UPDATE user_credentials SET expires_at = 0", CodeOnlySelect},
		{"SELECT * FROM (DELETE FROM user_credentials) t", CodeDMLNotAllowed},
	}
	for _, tc := range cases {
		_, err := newTestGuard().Validate(tc.sql)
		if code := codeOf(t, err); code != tc.code {
			t.Fatalf("%q 期望先报 %s，得到 %s", tc.sql, tc.code, code)
		}
	}
}

func TestNewGuardDeduplicatesAndLowers(t *testing.T) {
	g := NewGuard([]string{"Iam_Users", "iam_users", "  ", "iam_sessions"})
	got := g.AllowedTables()
	if len(got) != 2 || got[0] != "iam_users" || got[1] != "iam_sessions" {
		t.Fatalf("白名单去重/小写失败: %v", got)
	}
}
