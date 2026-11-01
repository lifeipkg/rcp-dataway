package com.risk.platform.dataway.web;

import java.util.List;

/**
 * 模板化写入请求。templateId 必须在服务端登记；不接受裸 DML。
 */
public record ExecuteRequest(String datasource, String templateId, List<Object> params) {

    public List<Object> paramsOrEmpty() {
        return params == null ? List.of() : params;
    }
}
