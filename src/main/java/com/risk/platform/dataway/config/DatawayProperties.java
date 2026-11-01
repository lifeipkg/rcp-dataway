package com.risk.platform.dataway.config;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.util.List;
import java.util.Map;

/**
 * 数据网关配置：数据源名、表白名单、写入模板。
 *
 * <p>白名单与模板都在服务端配置，客户端只能引用名字，不能传连接串或自定义写入语句。
 */
@ConfigurationProperties(prefix = "dataway")
public record DatawayProperties(
        String datasourceName,
        List<String> allowedTables,
        Map<String, String> templates,
        int defaultMaxRows) {

    public DatawayProperties {
        allowedTables = allowedTables == null ? List.of() : List.copyOf(allowedTables);
        templates = templates == null ? Map.of() : Map.copyOf(templates);
        datasourceName = datasourceName == null ? "main" : datasourceName;
        defaultMaxRows = defaultMaxRows <= 0 ? 100 : defaultMaxRows;
    }
}
