// Package guard 是数据网关的两道闸：只读查询过 SQL 守卫，写入只认服务端模板。
//
// 这里的判定全部可机械复核（正则 + 前缀 + 白名单集合），不追求 SQL 语义分析。
// 做不到的事写进 README 的"已知边界"，不在代码里假装做到。
package guard

import "fmt"

// 拒绝码：客户端与审计按 code 分类，message 只用于人读，不参与判定。
const (
	CodeEmptySQL         = "EMPTY_SQL"
	CodeMultiStatement   = "MULTI_STATEMENT"
	CodeCommentNotAllowed = "COMMENT_NOT_ALLOWED"
	CodeOnlySelect       = "ONLY_SELECT"
	CodeDMLNotAllowed    = "DML_NOT_ALLOWED"
	CodeTableNotAllowed  = "TABLE_NOT_ALLOWED"
	CodeDatasourceDenied = "DATASOURCE_NOT_ALLOWED"
	CodeTemplateNotFound = "TEMPLATE_NOT_FOUND"
	CodeParamMismatch    = "PARAM_MISMATCH"
)

// Rejected 是"请求被守卫拒绝"，对应 HTTP 400 + 编码化原因。
//
// 它只用于可预期的拒绝；数据库连不上之类的故障不是它，不能伪装成 400。
type Rejected struct {
	Code    string
	Message string
}

func (e *Rejected) Error() string { return e.Message }

func reject(code, format string, args ...any) *Rejected {
	return &Rejected{Code: code, Message: fmt.Sprintf(format, args...)}
}
