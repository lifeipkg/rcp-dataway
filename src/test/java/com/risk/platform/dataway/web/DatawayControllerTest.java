package com.risk.platform.dataway.web;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.test.web.servlet.MockMvc;

import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest(properties = "spring.cloud.nacos.discovery.enabled=false")
@AutoConfigureMockMvc
class DatawayControllerTest {

    @Autowired
    private MockMvc mvc;

    @Autowired
    private JdbcTemplate jdbc;

    @BeforeEach
    void setUpSchema() {
        // 测试夹具：表结构真源在 rcp-ops/migrations，这里只为跑通路由建最小表
        jdbc.execute("CREATE TABLE IF NOT EXISTS patrol_verdict ("
                + "event_id VARCHAR(64) PRIMARY KEY, decision VARCHAR(16), score INT)");
        jdbc.execute("CREATE TABLE IF NOT EXISTS patrol_event ("
                + "event_id VARCHAR(64) PRIMARY KEY, domain VARCHAR(16), version INT)");
        jdbc.update("DELETE FROM patrol_verdict");
        jdbc.update("DELETE FROM patrol_event");
    }

    @Test
    @DisplayName("模板写入后可按白名单查询到")
    void executeThenQuery() throws Exception {
        mvc.perform(post("/dataway/execute").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"datasource\":\"main\",\"templateId\":\"verdict.insert\","
                                + "\"params\":[\"job:1\",\"INTERCEPT\",80]}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.affectedRows").value(1));

        mvc.perform(post("/dataway/query").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"datasource\":\"main\","
                                + "\"sql\":\"SELECT event_id, decision, score FROM patrol_verdict WHERE event_id = ?\","
                                + "\"params\":[\"job:1\"]}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.rowCount").value(1))
                .andExpect(jsonPath("$.rows[0].decision").value("INTERCEPT"))
                .andExpect(jsonPath("$.rows[0].score").value(80));
    }

    @Test
    @DisplayName("未登记模板拒绝")
    void unknownTemplateRejected() throws Exception {
        mvc.perform(post("/dataway/execute").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"datasource\":\"main\",\"templateId\":\"raw.sql\",\"params\":[]}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error").value("TEMPLATE_NOT_FOUND"));
    }

    @Test
    @DisplayName("参数个数不匹配拒绝")
    void paramMismatchRejected() throws Exception {
        mvc.perform(post("/dataway/execute").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"datasource\":\"main\",\"templateId\":\"verdict.insert\",\"params\":[\"job:1\"]}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error").value("PARAM_MISMATCH"));
    }

    @Test
    @DisplayName("查询里的裸 DML 拒绝")
    void rawDmlThroughQueryRejected() throws Exception {
        mvc.perform(post("/dataway/query").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"datasource\":\"main\","
                                + "\"sql\":\"DELETE FROM patrol_verdict\"}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error").value("ONLY_SELECT"));
    }

    @Test
    @DisplayName("未登记的数据源拒绝")
    void unknownDatasourceRejected() throws Exception {
        mvc.perform(post("/dataway/query").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"datasource\":\"shadow\",\"sql\":\"SELECT 1 FROM patrol_event\"}"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.error").value("DATASOURCE_NOT_ALLOWED"));
    }

    @Test
    @DisplayName("maxRows 生效")
    void maxRowsApplied() throws Exception {
        jdbc.update("INSERT INTO patrol_event(event_id, domain, version) VALUES ('a','content',1)");
        jdbc.update("INSERT INTO patrol_event(event_id, domain, version) VALUES ('b','fund',1)");

        mvc.perform(post("/dataway/query").contentType(MediaType.APPLICATION_JSON)
                        .content("{\"datasource\":\"main\",\"sql\":\"SELECT event_id FROM patrol_event\","
                                + "\"maxRows\":1}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.rowCount").value(1));
    }
}
