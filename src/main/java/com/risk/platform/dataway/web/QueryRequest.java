package com.risk.platform.dataway.web;

import java.util.List;

/**
 * 只读查询请求。sql 必须通过 SqlGuard；datasource 只能引用服务端已配置的名字。
 */
public record QueryRequest(String datasource, String sql, List<Object> params, Integer maxRows) {

    public List<Object> paramsOrEmpty() {
        return params == null ? List.of() : params;
    }
}
