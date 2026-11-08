package guard

import (
	"strconv"
	"strings"
)

// Rewrite 把 JDBC 风格的 ? 位置参数改写成 PostgreSQL 的 $n。
//
// 契约里的 SQL 是 ? 风格（调用方按 OpenAI 无关的既有习惯传参），而 PostgreSQL
// 只认 $n；转换必须在网关内部完成，不能要求每个调用方自己数编号。
//
// 跳过单引号字符串、双引号标识符与 '' / "" 转义：`WHERE name = 'a?b'` 里的 ?
// 是数据，不是占位符。只做这一件事，不做语法分析。
func Rewrite(sql string) string {
	rewritten, _ := Params(sql)
	return rewritten
}

// Params 在同一遍扫描里返回改写后的语句与参数个数。
//
// 参数个数与改写结果同源，避免"数 ? 的个数"和"替换 ?"两处逻辑各自演化后
// 出现"数出 3 个、只替换 2 个"这种只在参数个数校验上才会暴露的错位。
func Params(sql string) (string, int) {
	var b strings.Builder
	b.Grow(len(sql) + 8)

	count := 0
	inSingle := false
	inDouble := false

	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case inSingle:
			b.WriteByte(c)
			if c == '\'' {
				// '' 是转义的单引号，仍在字符串内
				if i+1 < len(sql) && sql[i+1] == '\'' {
					b.WriteByte(sql[i+1])
					i++
					continue
				}
				inSingle = false
			}
		case inDouble:
			b.WriteByte(c)
			if c == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' {
					b.WriteByte(sql[i+1])
					i++
					continue
				}
				inDouble = false
			}
		default:
			switch c {
			case '\'':
				inSingle = true
				b.WriteByte(c)
			case '"':
				inDouble = true
				b.WriteByte(c)
			case '?':
				count++
				b.WriteByte('$')
				b.WriteString(strconv.Itoa(count))
			default:
				b.WriteByte(c)
			}
		}
	}
	return b.String(), count
}
