package com.risk.platform.dataway.guard;

import com.risk.platform.dataway.config.DatawayProperties;
import org.springframework.stereotype.Component;

import java.util.LinkedHashSet;
import java.util.Locale;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * SQL 守卫：只放行"单条、只读、只引用白名单表"的查询。
 *
 * <p>这一层不追求完整 SQL 语法分析（那需要真正的解析器），而是用一组可机械复核的拒绝规则
 * 把常见越权路径堵死，并把拒绝原因编码化，便于审计与测试断言：
 * <ul>
 *   <li>EMPTY_SQL —— 空语句</li>
 *   <li>MULTI_STATEMENT —— 出现多个分号，防"SELECT 1; DROP ..."</li>
 *   <li>COMMENT_NOT_ALLOWED —— 出现注释，防用注释截断审计与拼接绕过</li>
 *   <li>ONLY_SELECT —— 不以 SELECT 开头</li>
 *   <li>DML_NOT_ALLOWED —— 语句内出现写入/DDL 关键字（含 CTE 里夹带写操作）</li>
 *   <li>TABLE_NOT_ALLOWED —— 引用了白名单外的表</li>
 * </ul>
 *
 * <p><b>已知边界</b>：动态 SQL 拼接、存储过程副作用不在本层拦截范围内，需靠数据源账号只读权限兜底。
 */
@Component
public class SqlGuard {

    private static final Pattern WRITE_KEYWORDS = Pattern.compile(
            "\\b(insert|update|delete|drop|alter|create|truncate|grant|revoke|merge|call|exec|execute|copy|vacuum)\\b",
            Pattern.CASE_INSENSITIVE);

    private static final Pattern TABLE_REF = Pattern.compile(
            "\\b(?:from|join)\\s+([\\w.\"`]+)",
            Pattern.CASE_INSENSITIVE);

    private final Set<String> allowedTables;

    public SqlGuard(DatawayProperties properties) {
        Set<String> tables = new LinkedHashSet<>();
        properties.allowedTables().forEach(t -> tables.add(t.toLowerCase(Locale.ROOT)));
        this.allowedTables = Set.copyOf(tables);
    }

    /** 校验并返回去掉尾分号的语句。 */
    public String validate(String sql) {
        if (sql == null || sql.isBlank()) {
            throw new SqlRejectedException("EMPTY_SQL", "SQL 为空");
        }
        String statement = sql.trim();
        if (statement.endsWith(";")) {
            statement = statement.substring(0, statement.length() - 1).trim();
        }
        if (statement.contains(";")) {
            throw new SqlRejectedException("MULTI_STATEMENT", "不允许多条语句");
        }
        if (statement.contains("--") || statement.contains("/*")) {
            throw new SqlRejectedException("COMMENT_NOT_ALLOWED", "不允许 SQL 注释");
        }
        if (!statement.regionMatches(true, 0, "SELECT", 0, 6)) {
            throw new SqlRejectedException("ONLY_SELECT", "只允许 SELECT 查询");
        }
        Matcher writeMatcher = WRITE_KEYWORDS.matcher(statement);
        if (writeMatcher.find()) {
            throw new SqlRejectedException("DML_NOT_ALLOWED",
                    "查询中出现写操作关键字: " + writeMatcher.group(1));
        }
        for (String table : referencedTables(statement)) {
            if (!allowedTables.contains(table)) {
                throw new SqlRejectedException("TABLE_NOT_ALLOWED",
                        "表不在白名单: " + table + "（允许: " + allowedTables + "）");
            }
        }
        return statement;
    }

    /** 抽取 FROM / JOIN 后的表名，去掉 schema 前缀与引号。 */
    static Set<String> referencedTables(String statement) {
        Set<String> tables = new LinkedHashSet<>();
        Matcher matcher = TABLE_REF.matcher(statement);
        while (matcher.find()) {
            String raw = matcher.group(1).replace("\"", "").replace("`", "");
            int dot = raw.lastIndexOf('.');
            tables.add((dot >= 0 ? raw.substring(dot + 1) : raw).toLowerCase(Locale.ROOT));
        }
        return tables;
    }

    public Set<String> allowedTables() {
        return allowedTables;
    }
}
