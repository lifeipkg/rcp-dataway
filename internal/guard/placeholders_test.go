package guard

import "testing"

func TestRewritePlaceholders(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want string
		n    int
	}{
		{
			name: "顺序编号",
			sql:  "SELECT * FROM iam_sessions WHERE session_id = ? AND expires_at > ?",
			want: "SELECT * FROM iam_sessions WHERE session_id = $1 AND expires_at > $2",
			n:    2,
		},
		{
			name: "无占位符",
			sql:  "SELECT 1 FROM iam_users",
			want: "SELECT 1 FROM iam_users",
			n:    0,
		},
		{
			name: "字符串字面量里的问号不替换",
			sql:  "SELECT * FROM iam_users WHERE kind = 'a?b' AND seq = ?",
			want: "SELECT * FROM iam_users WHERE kind = 'a?b' AND seq = $1",
			n:    1,
		},
		{
			name: "转义单引号后仍是字符串",
			sql:  "SELECT * FROM iam_users WHERE kind = 'it''s ?' AND seq = ?",
			want: "SELECT * FROM iam_users WHERE kind = 'it''s ?' AND seq = $1",
			n:    1,
		},
		{
			name: "双引号标识符里的问号不替换",
			sql:  `SELECT * FROM iam_users WHERE "we?ird" = ?`,
			want: `SELECT * FROM iam_users WHERE "we?ird" = $1`,
			n:    1,
		},
		{
			name: "模板里的 CAST 占位符",
			sql:  "INSERT INTO iam_sessions(token, user_id, expires_at) VALUES (?, ?, CAST(? AS TIMESTAMP))",
			want: "INSERT INTO iam_sessions(token, user_id, expires_at) VALUES ($1, $2, CAST($3 AS TIMESTAMP))",
			n:    3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, n := Params(tc.sql)
			if got != tc.want {
				t.Fatalf("改写结果不符\n得到: %s\n期望: %s", got, tc.want)
			}
			if n != tc.n {
				t.Fatalf("参数个数期望 %d，得到 %d", tc.n, n)
			}
		})
	}
}

func TestRewriteIsIdempotentOnDollarStyle(t *testing.T) {
	// 已经是 $n 的语句不应被再改写（客户端偶发传 $n 时不制造双重编号）
	in := "SELECT * FROM iam_users WHERE session_id = $1"
	if got := Rewrite(in); got != in {
		t.Fatalf("$n 风格语句被改写: %s", got)
	}
}
