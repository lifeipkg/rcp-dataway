package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lifeipkg/rcp-dataway/internal/guard"
	"github.com/lifeipkg/rcp-dataway/internal/store"
)

// fakeDB 记录网关最终发给数据库的 SQL 与参数：拒绝路径与改写结果都能在没有
// PostgreSQL 的环境里被断言。生产实现见 internal/store。
type fakeDB struct {
	rows      []map[string]any
	affected  int64
	querySQL  string
	queryArgs []any
	queryMax  int
	execSQL   string
	execArgs  []any
	queryErr  error
	execErr   error
	pingErr   error
}

func (f *fakeDB) Query(_ context.Context, sql string, args []any, maxRows int) (store.Result, error) {
	f.querySQL = sql
	f.queryArgs = args
	f.queryMax = maxRows
	if f.queryErr != nil {
		return store.Result{}, f.queryErr
	}
	rows := f.rows
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return store.Result{Columns: []string{"session_id", "status"}, Rows: rows}, nil
}

func (f *fakeDB) Exec(_ context.Context, sql string, args []any) (int64, error) {
	f.execSQL = sql
	f.execArgs = args
	if f.execErr != nil {
		return 0, f.execErr
	}
	return f.affected, nil
}

func (f *fakeDB) Ping(context.Context) error { return f.pingErr }
func (f *fakeDB) Close()                     {}

func newTestRouter(t *testing.T, db store.DB) http.Handler {
	t.Helper()
	templates, err := guard.NewTemplates(map[string]string{
		"iam.session.insert": "INSERT INTO iam_sessions(session_id, user_id, expires_at) VALUES (?, ?, ?)",
	})
	if err != nil {
		t.Fatalf("模板登记失败: %v", err)
	}
	g := guard.NewGuard([]string{"iam_users", "iam_sessions"})
	return NewRouter(NewHandler(db, g, templates, "main", 100))
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON: %v（body=%s）", err, rec.Body.String())
	}
	return out
}

func TestExecuteThenQuery(t *testing.T) {
	db := &fakeDB{
		affected: 1,
		rows:     []map[string]any{{"session_id": "s-1", "status": "ACTIVE"}},
	}
	h := newTestRouter(t, db)

	rec := post(t, h, "/dataway/execute",
		`{"datasource":"main","templateId":"iam.session.insert","params":["s-1","ACTIVE",80]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("模板写入期望 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode(t, rec)["affectedRows"]; got != float64(1) {
		t.Fatalf("affectedRows 期望 1，得到 %v", got)
	}
	// 写入必须走改写后的语句：JDBC 的 ? 不能直接发给 PostgreSQL
	if want := "INSERT INTO iam_sessions(session_id, user_id, expires_at) VALUES ($1, $2, $3)"; db.execSQL != want {
		t.Fatalf("写入语句未改写\n得到: %s\n期望: %s", db.execSQL, want)
	}

	rec = post(t, h, "/dataway/query",
		`{"datasource":"main","sql":"SELECT session_id, status FROM iam_sessions WHERE session_id = ?","params":["s-1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("查询期望 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["rowCount"] != float64(1) {
		t.Fatalf("rowCount 期望 1，得到 %v", body["rowCount"])
	}
	rows, ok := body["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("rows 形状不符: %v", body["rows"])
	}
	if got := rows[0].(map[string]any)["status"]; got != "ACTIVE" {
		t.Fatalf("status 期望 ACTIVE，得到 %v", got)
	}
	if !strings.Contains(db.querySQL, "$1") {
		t.Fatalf("查询语句未改写成 $n: %s", db.querySQL)
	}
}

func TestRejections(t *testing.T) {
	cases := []struct {
		name string
		path string
		body string
		code string
	}{
		{
			name: "未登记模板",
			path: "/dataway/execute",
			body: `{"datasource":"main","templateId":"raw.sql","params":[]}`,
			code: guard.CodeTemplateNotFound,
		},
		{
			name: "参数个数不匹配",
			path: "/dataway/execute",
			body: `{"datasource":"main","templateId":"iam.session.insert","params":["s-1"]}`,
			code: guard.CodeParamMismatch,
		},
		{
			name: "查询里的裸 DML",
			path: "/dataway/query",
			body: `{"datasource":"main","sql":"DELETE FROM iam_sessions"}`,
			code: guard.CodeOnlySelect,
		},
		{
			name: "未登记的数据源",
			path: "/dataway/query",
			body: `{"datasource":"shadow","sql":"SELECT 1 FROM iam_users"}`,
			code: guard.CodeDatasourceDenied,
		},
		{
			name: "白名单外的表",
			path: "/dataway/query",
			body: `{"datasource":"main","sql":"SELECT * FROM user_credentials"}`,
			code: guard.CodeTableNotAllowed,
		},
		{
			name: "请求体不是 JSON",
			path: "/dataway/query",
			body: `{`,
			code: "BAD_REQUEST",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestRouter(t, &fakeDB{})
			rec := post(t, h, tc.path, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("期望 400，得到 %d: %s", rec.Code, rec.Body.String())
			}
			if got := decode(t, rec)["error"]; got != tc.code {
				t.Fatalf("期望 code %s，得到 %v", tc.code, got)
			}
		})
	}
}

func TestRejectionDoesNotTouchDatabase(t *testing.T) {
	db := &fakeDB{}
	h := newTestRouter(t, db)
	post(t, h, "/dataway/query", `{"datasource":"main","sql":"DELETE FROM iam_sessions"}`)
	if db.querySQL != "" || db.execSQL != "" {
		t.Fatalf("被拒绝的请求不应到达数据库层: query=%q exec=%q", db.querySQL, db.execSQL)
	}
}

func TestMaxRowsApplied(t *testing.T) {
	db := &fakeDB{rows: []map[string]any{
		{"session_id": "a"}, {"session_id": "b"},
	}}
	h := newTestRouter(t, db)

	rec := post(t, h, "/dataway/query",
		`{"datasource":"main","sql":"SELECT session_id FROM iam_users","maxRows":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", rec.Code)
	}
	if got := decode(t, rec)["rowCount"]; got != float64(1) {
		t.Fatalf("rowCount 期望 1，得到 %v", got)
	}
	if db.queryMax != 1 {
		t.Fatalf("maxRows 应传给数据库层，得到 %d", db.queryMax)
	}
}

func TestMaxRowsCannotExceedServerDefault(t *testing.T) {
	db := &fakeDB{}
	h := newTestRouter(t, db) // 服务端默认 100
	post(t, h, "/dataway/query",
		`{"datasource":"main","sql":"SELECT session_id FROM iam_users","maxRows":1000}`)
	if db.queryMax != 100 {
		t.Fatalf("客户端 maxRows 不得突破服务端默认 100，得到 %d", db.queryMax)
	}
}

func TestDatasourceDefaultsWhenOmitted(t *testing.T) {
	db := &fakeDB{}
	h := newTestRouter(t, db)
	rec := post(t, h, "/dataway/query", `{"sql":"SELECT session_id FROM iam_users"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("缺省 datasource 应落到服务端配置名，得到 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInternalErrorIsNotLeaked(t *testing.T) {
	db := &fakeDB{queryErr: errors.New(`relation "iam_users" does not exist`)}
	h := newTestRouter(t, db)
	rec := post(t, h, "/dataway/query", `{"datasource":"main","sql":"SELECT session_id FROM iam_users"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("数据库故障期望 500，得到 %d", rec.Code)
	}
	body := decode(t, rec)
	if body["error"] != "INTERNAL_ERROR" {
		t.Fatalf("期望 INTERNAL_ERROR，得到 %v", body["error"])
	}
	if strings.Contains(rec.Body.String(), "iam_users") {
		t.Fatalf("数据库错误细节不应回给调用方: %s", rec.Body.String())
	}
}

func TestHealthReflectsDatabase(t *testing.T) {
	h := newTestRouter(t, &fakeDB{})
	req := httptest.NewRequest(http.MethodGet, "/actuator/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || decode(t, rec)["status"] != "UP" {
		t.Fatalf("数据库可用时期望 UP，得到 %d: %s", rec.Code, rec.Body.String())
	}

	down := newTestRouter(t, &fakeDB{pingErr: errors.New("connection refused")})
	rec = httptest.NewRecorder()
	down.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/actuator/health", nil))
	if rec.Code != http.StatusServiceUnavailable || decode(t, rec)["status"] != "DOWN" {
		t.Fatalf("数据库不可用时期望 503 DOWN，得到 %d: %s", rec.Code, rec.Body.String())
	}
}
