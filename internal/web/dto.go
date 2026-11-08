package web

// QueryRequest 是只读查询请求。sql 必须通过守卫；datasource 只能引用服务端已配置的名字。
//
// MaxRows 用指针：契约里 maxRows 缺省与给 0 是同一件事（都用服务端默认），
// 用值类型会把"没传"当成"要 0 行"。
type QueryRequest struct {
	Datasource string `json:"datasource"`
	SQL        string `json:"sql"`
	Params     []any  `json:"params"`
	MaxRows    *int   `json:"maxRows"`
}

// ExecuteRequest 是模板化写入请求。templateId 必须在服务端登记；不接受裸 DML。
type ExecuteRequest struct {
	Datasource string `json:"datasource"`
	TemplateID string `json:"templateId"`
	Params     []any  `json:"params"`
}

type queryResponse struct {
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"rowCount"`
	ElapsedMs int64            `json:"elapsedMs"`
}

type executeResponse struct {
	AffectedRows int64 `json:"affectedRows"`
}

// errorResponse 是拒绝与故障的统一形状：code 供程序分支，message 供人读。
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
