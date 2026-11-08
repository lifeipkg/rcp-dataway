package guard

import (
	"sort"
	"strings"
)

// Templates 是写入模板登记表。客户端只传模板 ID 与参数，不传 SQL。
//
// Resolve 直接把模板改写成 $n 并同时给出参数个数：调用方拿到的是"可执行语句 +
// 期望参数个数"这一对，不存在"忘了改写占位符"或"数错了参数"的中间态。
type Templates struct {
	byID  map[string]string
	order []string
}

// NewTemplates 逐条校验模板形状；任何一条不合规都整体失败。
//
// 不做"跳过坏模板继续启动"：少一条模板会让某条写入路径在运行时才 400，
// 而启动时它是能查出来的。
func NewTemplates(configured map[string]string) (*Templates, error) {
	byID := make(map[string]string, len(configured))
	order := make([]string, 0, len(configured))
	for id, sql := range configured {
		if err := ValidateTemplate(id, sql); err != nil {
			return nil, err
		}
		byID[id] = sql
		order = append(order, id)
	}
	sort.Strings(order)
	return &Templates{byID: byID, order: order}, nil
}

// Resolve 返回改写后的 SQL 与模板要求的参数个数。
func (t *Templates) Resolve(id string) (string, int, error) {
	sql, ok := t.byID[id]
	if !ok {
		return "", 0, reject(CodeTemplateNotFound, "模板未登记: %s（已登记: %s）",
			id, strings.Join(t.order, ", "))
	}
	rewritten, count := Params(sql)
	return rewritten, count, nil
}

// IDs 返回已登记模板 ID（排序后），用于错误信息与运维排查。
func (t *Templates) IDs() []string {
	out := make([]string, len(t.order))
	copy(out, t.order)
	return out
}
