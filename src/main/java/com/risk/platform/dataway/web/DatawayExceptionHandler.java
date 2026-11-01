package com.risk.platform.dataway.web;

import com.risk.platform.dataway.guard.SqlRejectedException;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;

import java.util.Map;

/**
 * 把守卫拒绝翻译成 400 + 编码化原因，方便调用方与审计按 code 分类。
 */
@RestControllerAdvice
public class DatawayExceptionHandler {

    @ExceptionHandler(SqlRejectedException.class)
    public ResponseEntity<Map<String, String>> onRejected(SqlRejectedException ex) {
        return ResponseEntity.status(HttpStatus.BAD_REQUEST)
                .body(Map.of("error", ex.code(), "message", ex.getMessage()));
    }
}
