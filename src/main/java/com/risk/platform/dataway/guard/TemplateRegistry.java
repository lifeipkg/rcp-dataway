package com.risk.platform.dataway.guard;

import com.risk.platform.dataway.config.DatawayProperties;
import org.springframework.stereotype.Component;

import java.util.Map;

/**
 * 写入模板登记表。客户端只能引用模板 ID，不能自带 SQL。
 *
 * <p>模板在 application.yml 里声明，随部署走；新增模板属于数据访问面变更，
 * 需要与契约（dataway.yaml）和调用方同步。
 */
@Component
public class TemplateRegistry {

    private final Map<String, String> templates;

    public TemplateRegistry(DatawayProperties properties) {
        this.templates = properties.templates();
    }

    public String resolve(String templateId) {
        String sql = templates.get(templateId);
        if (sql == null) {
            throw new SqlRejectedException("TEMPLATE_NOT_FOUND",
                    "模板未登记: " + templateId + "（已登记: " + templates.keySet() + "）");
        }
        return sql;
    }

    public int requiredParams(String sql) {
        int count = 0;
        for (char ch : sql.toCharArray()) {
            if (ch == '?') {
                count++;
            }
        }
        return count;
    }

    public Map<String, String> all() {
        return templates;
    }
}
