package guard

import (
	"regexp"
	"strings"
)

var (
	// 写/DDL 关键字：出现在 SELECT 里就是夹带，包括 CTE 内部。
	writeKeywords = regexp.MustCompile(
		`(?i)\b(insert|update|delete|drop|alter|create|truncate|grant|revoke|merge|call|exec|execute|copy|vacuum)\b`)

	// FROM / JOIN 之后的表引用；schema 前缀与引号在抽取后去掉。
	tableRef = regexp.MustCompile("(?i)\\b(?:from|join)\\s+([\\w.\"`]+)")

	// 模板必须是单条 DML：写入面只从这里进，入口形状先卡死。
	writeLeading = regexp.MustCompile(`(?i)^(insert|update|delete)\b`)
)

// Guard 只放行"单条、只读、只引用白名单表"的查询。
//
// 拒绝原因编码化，按判定顺序：EMPTY_SQL → MULTI_STATEMENT → COMMENT_NOT_ALLOWED
// → ONLY_SELECT → DML_NOT_ALLOWED → TABLE_NOT_ALLOWED。顺序是契约的一部分：
// 同一句 SQL 只报第一个命中的 code，调用方不需要猜优先级。
type Guard struct {
	allowed map[string]struct{}
	order   []string
}

// NewGuard 构造守卫；表名统一小写，避免白名单因大小写写成两套。
func NewGuard(tables []string) *Guard {
	allowed := make(map[string]struct{}, len(tables))
	order := make([]string, 0, len(tables))
	for _, table := range tables {
		name := strings.ToLower(strings.TrimSpace(table))
		if name == "" {
			continue
		}
		if _, exists := allowed[name]; exists {
			continue
		}
		allowed[name] = struct{}{}
		order = append(order, name)
	}
	return &Guard{allowed: allowed, order: order}
}

// AllowedTables 返回白名单（按配置顺序），用于拒绝信息里的可读说明。
func (g *Guard) AllowedTables() []string {
	out := make([]string, len(g.order))
	copy(out, g.order)
	return out
}

// Validate 校验并返回去掉尾分号的语句。
func (g *Guard) Validate(sql string) (string, error) {
	if strings.TrimSpace(sql) == "" {
		return "", reject(CodeEmptySQL, "SQL 为空")
	}
	statement := strings.TrimSpace(sql)
	if strings.HasSuffix(statement, ";") {
		statement = strings.TrimSpace(statement[:len(statement)-1])
	}
	if strings.Contains(statement, ";") {
		return "", reject(CodeMultiStatement, "不允许多条语句")
	}
	if strings.Contains(statement, "--") || strings.Contains(statement, "/*") {
		return "", reject(CodeCommentNotAllowed, "不允许 SQL 注释")
	}
	if len(statement) < 6 || !strings.EqualFold(statement[:6], "SELECT") {
		return "", reject(CodeOnlySelect, "只允许 SELECT 查询")
	}
	if hit := writeKeywords.FindStringSubmatch(statement); hit != nil {
		return "", reject(CodeDMLNotAllowed, "查询中出现写操作关键字: %s", strings.ToLower(hit[1]))
	}
	for _, table := range referencedTables(statement) {
		if _, ok := g.allowed[table]; !ok {
			return "", reject(CodeTableNotAllowed, "表不在白名单: %s（允许: %s）",
				table, strings.Join(g.order, ", "))
		}
	}
	return statement, nil
}

// referencedTables 抽取 FROM / JOIN 后的表名，去掉 schema 前缀与引号。
func referencedTables(statement string) []string {
	seen := make(map[string]struct{})
	var tables []string
	for _, match := range tableRef.FindAllStringSubmatch(statement, -1) {
		raw := strings.NewReplacer(`"`, "", "`", "").Replace(match[1])
		if dot := strings.LastIndex(raw, "."); dot >= 0 {
			raw = raw[dot+1:]
		}
		name := strings.ToLower(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		tables = append(tables, name)
	}
	return tables
}

// ValidateTemplate 在启动时校验写入模板的形状。
//
// 模板来自服务端配置而不是请求，这里不是防攻击，是防配置退化：一条带分号或
// 注释的模板会让"模板即最小写入面"这句话不成立，而这种错配只会在生产第一次
// 调用时才暴露。宁可启动失败。
func ValidateTemplate(id, sql string) error {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return &Rejected{Code: CodeTemplateNotFound, Message: "模板 " + id + " 是空语句"}
	}
	if strings.Contains(trimmed, ";") {
		return &Rejected{Code: CodeMultiStatement,
			Message: "模板 " + id + " 含分号：模板只允许单条语句"}
	}
	if strings.Contains(trimmed, "--") || strings.Contains(trimmed, "/*") {
		return &Rejected{Code: CodeCommentNotAllowed,
			Message: "模板 " + id + " 含 SQL 注释"}
	}
	if !writeLeading.MatchString(trimmed) {
		return &Rejected{Code: CodeDMLNotAllowed,
			Message: "模板 " + id + " 不是单条 INSERT/UPDATE/DELETE：写入面只接受这三类"}
	}
	return nil
}
