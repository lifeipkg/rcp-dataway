package com.risk.platform.dataway.guard;

import com.risk.platform.dataway.config.DatawayProperties;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class SqlGuardTest {

    private final SqlGuard guard = new SqlGuard(new DatawayProperties(
            "main",
            List.of("patrol_event", "patrol_verdict"),
            Map.of(),
            100));

    @Test
    @DisplayName("白名单单表 SELECT 放行，并去掉尾分号")
    void allowsWhitelistedSelect() {
        assertEquals("SELECT * FROM patrol_verdict WHERE event_id = ?",
                guard.validate("  SELECT * FROM patrol_verdict WHERE event_id = ? ;  "));
    }

    @Test
    @DisplayName("join 白名单表放行")
    void allowsJoinOnWhitelistedTables() {
        String sql = "SELECT e.event_id FROM patrol_event e JOIN patrol_verdict v ON e.event_id = v.event_id";
        assertTrue(guard.validate(sql).contains("patrol_event"));
    }

    @Test
    @DisplayName("空语句拒绝")
    void rejectsEmpty() {
        assertCode("EMPTY_SQL", () -> guard.validate("  "));
    }

    @Test
    @DisplayName("多语句拒绝：分号后夹带 DDL")
    void rejectsMultiStatement() {
        assertCode("MULTI_STATEMENT", () -> guard.validate("SELECT 1 FROM patrol_event; DROP TABLE patrol_event"));
    }

    @Test
    @DisplayName("注释拒绝：防注释截断审计")
    void rejectsComment() {
        assertCode("COMMENT_NOT_ALLOWED", () -> guard.validate("SELECT * FROM patrol_event -- 注释"));
    }

    @Test
    @DisplayName("非 SELECT 拒绝")
    void rejectsNonSelect() {
        assertCode("ONLY_SELECT", () -> guard.validate("UPDATE patrol_verdict SET score = 0"));
    }

    @Test
    @DisplayName("CTE 中夹带写操作拒绝")
    void rejectsWriteInsideCte() {
        assertCode("DML_NOT_ALLOWED",
                () -> guard.validate("SELECT * FROM (DELETE FROM patrol_event RETURNING event_id) t"));
    }

    @Test
    @DisplayName("白名单外的表拒绝，并说明允许范围")
    void rejectsUnknownTable() {
        SqlRejectedException ex = assertThrows(SqlRejectedException.class,
                () -> guard.validate("SELECT * FROM user_credentials"));
        assertEquals("TABLE_NOT_ALLOWED", ex.code());
        assertTrue(ex.getMessage().contains("patrol_verdict"));
    }

    @Test
    @DisplayName("带 schema 前缀的表按最后一段判定")
    void stripsSchemaPrefix() {
        assertThrows(SqlRejectedException.class,
                () -> guard.validate("SELECT * FROM public.sys_user"));
    }

    private void assertCode(String expected, Runnable action) {
        SqlRejectedException ex = assertThrows(SqlRejectedException.class, action::run);
        assertEquals(expected, ex.code());
    }
}
