package com.risk.platform.dataway.web;

import com.risk.platform.dataway.config.DatawayProperties;
import com.risk.platform.dataway.guard.SqlGuard;
import com.risk.platform.dataway.guard.SqlRejectedException;
import com.risk.platform.dataway.guard.TemplateRegistry;
import jakarta.validation.Valid;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.sql.ResultSetMetaData;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

/**
 * 数据网关入口：POST /dataway/query 与 POST /dataway/execute。
 *
 * <p>业务服务不持 JDBC 驱动，所有读写经此。查询走守卫，写入走模板，两条路径都不接受客户端
 * 自带的可执行语句。
 */
@RestController
@RequestMapping("/dataway")
public class DatawayController {

    private final JdbcTemplate jdbc;
    private final SqlGuard guard;
    private final TemplateRegistry templates;
    private final DatawayProperties properties;

    public DatawayController(JdbcTemplate jdbc, SqlGuard guard,
                             TemplateRegistry templates, DatawayProperties properties) {
        this.jdbc = jdbc;
        this.guard = guard;
        this.templates = templates;
        this.properties = properties;
    }

    @PostMapping("/query")
    public Map<String, Object> query(@Valid @RequestBody QueryRequest request) {
        checkDatasource(request.datasource());
        String sql = guard.validate(request.sql());
        int maxRows = request.maxRows() == null || request.maxRows() <= 0
                ? properties.defaultMaxRows()
                : Math.min(request.maxRows(), properties.defaultMaxRows());

        long start = System.nanoTime();
        QueryResult result = jdbc.query(sql, rs -> {
            ResultSetMetaData meta = rs.getMetaData();
            int columnCount = meta.getColumnCount();
            List<String> columns = new ArrayList<>(columnCount);
            for (int i = 1; i <= columnCount; i++) {
                // 列名统一小写：跨数据库（H2/PG/MySQL）大小写规则不一致，网关出口统一口径
                columns.add(meta.getColumnLabel(i).toLowerCase(Locale.ROOT));
            }
            List<Map<String, Object>> rows = new ArrayList<>();
            while (rs.next() && rows.size() < maxRows) {
                Map<String, Object> row = new LinkedHashMap<>();
                for (int i = 1; i <= columnCount; i++) {
                    row.put(columns.get(i - 1), rs.getObject(i));
                }
                rows.add(row);
            }
            return new QueryResult(columns, rows);
        }, request.paramsOrEmpty().toArray());

        long elapsedMs = (System.nanoTime() - start) / 1_000_000;
        return Map.of(
                "columns", result.columns(),
                "rows", result.rows(),
                "rowCount", result.rows().size(),
                "elapsedMs", elapsedMs);
    }

    @PostMapping("/execute")
    public Map<String, Object> execute(@Valid @RequestBody ExecuteRequest request) {
        checkDatasource(request.datasource());
        String sql = templates.resolve(request.templateId());
        int required = templates.requiredParams(sql);
        int provided = request.paramsOrEmpty().size();
        if (required != provided) {
            throw new SqlRejectedException("PARAM_MISMATCH",
                    "模板 " + request.templateId() + " 需要 " + required + " 个参数，实际 " + provided);
        }
        int affected = jdbc.update(sql, request.paramsOrEmpty().toArray());
        return Map.of("affectedRows", affected);
    }

    private void checkDatasource(String datasource) {
        String requested = datasource == null || datasource.isBlank()
                ? properties.datasourceName()
                : datasource;
        if (!properties.datasourceName().equals(requested)) {
            throw new SqlRejectedException("DATASOURCE_NOT_ALLOWED",
                    "数据源未登记: " + requested + "（允许: " + properties.datasourceName() + "）");
        }
    }

    private record QueryResult(List<String> columns, List<Map<String, Object>> rows) {
    }
}
