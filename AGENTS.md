# AGENTS.md · rcp-dataway

本仓规则。工作区级流程见 `<工作区根>/AGENTS.md`。

## 本仓是什么

业务服务访问数据库的唯一出口（Go 1.22 + Gin + pgx，端口 8090）。它是**权限与数据安全的高风险仓**：
白名单、模板、守卫规则任何一处放宽，都等于给全部业务服务开了一个裸 SQL 口子。
早期 Java/Spring Boot 实现已退役，服务名、端口、契约与两道闸语义不变。

| 项 | 值 |
|---|---|
| 服务名 / 端口 | `dataway` / `8090` |
| 契约 | `rcp-contracts/contracts/openapi/dataway.yaml` |
| 技术栈 | Go 1.22 + Gin 1.10.0 + pgx v5（版本真源：`rcp-ontology/ontology/abox/platform.ttl` 的 `rcp:stack`） |
| 存储 | PostgreSQL，连接串只在本进程内 |

## 硬约束

1. **不放宽守卫换便利。** 需要新的查询形态，先想清楚是否真的需要；确实需要就补白名单表/模板并配一条反向测试（拒绝路径），不允许临时加 `allowAll` 开关。
2. **白名单是表级最小集。** 只加业务确实要读的表；用户凭据、支付流水这类表默认不进白名单。
3. **写入只能走模板。** 任何"直接传 UPDATE"的需求都属于契约变更，先改 `dataway.yaml` 与调用方，再改实现。模板必须单条 `INSERT/UPDATE/DELETE`；启动时校验，不合规即不启动。
4. **不给业务服务直连。** 业务仓的 `go.mod` / `pom.xml` 引入数据库驱动即 RL-005 红（检查器两边都扫）；发现直连先拆掉再谈功能。只有本仓与 `rcp-db-ops` 允许持有驱动。
5. **边界写进 README。** 守卫不是 SQL 解析器，做不到的事要明说，不许在文档里暗示"全面防注入"。
6. **路由写成完整字面量。** `rcp-contracts/tools/contract_check.py` 用正则抽 `r.POST("/dataway/...")` 与 OpenAPI 做两向差集，变量拼接或 `Group()` 相对路径会让机械对表失效；探活 `/actuator/health` 用 `r.Handle` 注册，不计入契约路由。
7. **占位符改写与参数计数同源。** `?` → `$n` 的改写与参数个数必须来自同一遍扫描（`guard.Params`），不允许两处各写一套正则。

## 验证

```bash
go vet ./... && go test ./... && go build ./...
python ../rcp-contracts/tools/contract_check.py --repo . \
  --contract ../rcp-contracts/contracts/openapi/dataway.yaml
```

新增一条拒绝规则必须同时新增一条断言该 code 的测试；只测放行路径等于没验证。
拒绝路径的测试用 `store.DB` 的 fake 实现，不需要真实数据库；真实库的连接时序不在单测覆盖范围内。
