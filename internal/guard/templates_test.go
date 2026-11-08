package guard

import (
	"errors"
	"testing"
)

func TestTemplatesResolveRewritesAndCounts(t *testing.T) {
	templates, err := NewTemplates(map[string]string{
		"verdict.insert": "INSERT INTO patrol_verdict(event_id, decision, score) VALUES (?, ?, ?)",
		"session.insert": "INSERT INTO iam_sessions(token, user_id, expires_at) VALUES (?, ?, CAST(? AS TIMESTAMP))",
	})
	if err != nil {
		t.Fatalf("合法模板应能登记: %v", err)
	}

	sql, count, err := templates.Resolve("verdict.insert")
	if err != nil {
		t.Fatalf("已登记模板应能解析: %v", err)
	}
	if want := "INSERT INTO patrol_verdict(event_id, decision, score) VALUES ($1, $2, $3)"; sql != want {
		t.Fatalf("模板未改写为 $n\n得到: %s\n期望: %s", sql, want)
	}
	if count != 3 {
		t.Fatalf("参数个数期望 3，得到 %d", count)
	}

	if _, count, _ := templates.Resolve("session.insert"); count != 3 {
		t.Fatalf("CAST 内占位符应计入参数个数，得到 %d", count)
	}
}

func TestTemplatesUnknownID(t *testing.T) {
	templates, err := NewTemplates(map[string]string{
		"event.insert": "INSERT INTO patrol_event(event_id) VALUES (?)",
	})
	if err != nil {
		t.Fatalf("模板登记失败: %v", err)
	}
	_, _, err = templates.Resolve("raw.sql")
	var rejected *Rejected
	if !errors.As(err, &rejected) {
		t.Fatalf("未登记模板应报 Rejected，得到 %v", err)
	}
	if rejected.Code != CodeTemplateNotFound {
		t.Fatalf("期望 %s，得到 %s", CodeTemplateNotFound, rejected.Code)
	}
}

func TestNewTemplatesRejectsBadShapes(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		code string
	}{
		{"空语句", "   ", CodeTemplateNotFound},
		{"含分号", "INSERT INTO t(a) VALUES (?); DROP TABLE t", CodeMultiStatement},
		{"含注释", "INSERT INTO t(a) VALUES (?) -- x", CodeCommentNotAllowed},
		{"不是 DML", "SELECT * FROM patrol_event", CodeDMLNotAllowed},
		{"DDL", "CREATE TABLE t(a INT)", CodeDMLNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewTemplates(map[string]string{"bad": tc.sql})
			var rejected *Rejected
			if !errors.As(err, &rejected) {
				t.Fatalf("非法模板应报 Rejected，得到 %v", err)
			}
			if rejected.Code != tc.code {
				t.Fatalf("期望 %s，得到 %s", tc.code, rejected.Code)
			}
		})
	}
}

func TestNewTemplatesRejectsEmptySet(t *testing.T) {
	templates, err := NewTemplates(nil)
	if err != nil {
		t.Fatalf("空集合本身不报错（由 Load 决定是否启动）: %v", err)
	}
	if ids := templates.IDs(); len(ids) != 0 {
		t.Fatalf("期望空模板集，得到 %v", ids)
	}
}
