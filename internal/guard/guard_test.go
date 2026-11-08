package guard

import (
	"errors"
	"strings"
	"testing"
)

func newTestGuard() *Guard {
	return NewGuard([]string{"patrol_event", "patrol_verdict"})
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
	got, err := newTestGuard().Validate("  SELECT * FROM patrol_verdict WHERE event_id = ? ;  ")
	if err != nil {
		t.Fatalf("白名单单表 SELECT 应放行: %v", err)
	}
	// 尾分号去掉、首尾空白去掉；占位符原样保留（改写是下一步）
	if want := "SELECT * FROM patrol_verdict WHERE event_id = ?"; got != want {
		t.Fatalf("语句规范化不符\n得到: %s\n期望: %s", got, want)
	}
}

func TestValidateAllowsJoinOnWhitelistedTables(t *testing.T) {
	sql := "SELECT e.event_id FROM patrol_event e JOIN patrol_verdict v ON e.event_id = v.event_id"
	if _, err := newTestGuard().Validate(sql); err != nil {
		t.Fatalf("白名单表 join 应放行: %v", err)
	}
}

func TestValidateAllowsLowerCaseSelectAndQuotedTable(t *testing.T) {
	for _, sql := range []string{
		"select * from patrol_event",
		`SELECT * FROM "patrol_verdict"`,
		"SELECT * FROM public.patrol_verdict",
		"SELECT (SELECT count(*) FROM patrol_event) AS c FROM patrol_verdict",
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
		{"多语句夹带 DDL", "SELECT 1 FROM patrol_event; DROP TABLE patrol_event", CodeMultiStatement},
		{"注释截断审计", "SELECT * FROM patrol_event -- 注释", CodeCommentNotAllowed},
		{"块注释", "SELECT /* x */ * FROM patrol_event", CodeCommentNotAllowed},
		{"非 SELECT", "UPDATE patrol_verdict SET score = 0", CodeOnlySelect},
		{"CTE 夹带写操作", "SELECT * FROM (DELETE FROM patrol_event RETURNING event_id) t", CodeDMLNotAllowed},
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
	if !strings.Contains(rejected.Message, "patrol_verdict") {
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
		{"DELETE /* c */ FROM patrol_event", CodeCommentNotAllowed},
		{"SELECT 1 FROM patrol_event; DROP TABLE patrol_event", CodeMultiStatement},
		{"UPDATE user_credentials SET score = 0", CodeOnlySelect},
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
	g := NewGuard([]string{"Patrol_Event", "patrol_event", "  ", "patrol_verdict"})
	got := g.AllowedTables()
	if len(got) != 2 || got[0] != "patrol_event" || got[1] != "patrol_verdict" {
		t.Fatalf("白名单去重/小写失败: %v", got)
	}
}
