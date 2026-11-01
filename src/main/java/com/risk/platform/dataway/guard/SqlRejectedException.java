package com.risk.platform.dataway.guard;

/**
 * 请求被 SQL 守卫拒绝。code 用于客户端区分是语法问题还是越权问题。
 */
public class SqlRejectedException extends RuntimeException {

    private final String code;

    public SqlRejectedException(String code, String message) {
        super(message);
        this.code = code;
    }

    public String code() {
        return code;
    }
}
