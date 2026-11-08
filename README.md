# rcp-dataway · 数据网关（Go）

业务服务访问数据库的**唯一出口**（红线 RL-005）。业务服务不持数据库驱动、不写裸 SQL。

| 项 | 值 |
|---|---|
| 服务名 | `dataway` |
| 端口 | `8090` |
| 技术栈 | Go 1.22 + Gin 1.10 + pgx v5（PostgreSQL） |
| 契约 | `rcp-contracts/contracts/openapi/dataway.yaml` |
| 探活 | `GET /actuator/health`（不在公开契约内） |

早期 Java/Spring Boot 实现在本次切换中退役，服务名、端口、契约与两道闸的语义不变；
变更面见文末「从 Java 切到 Go 的破坏性变更」。

## 两道闸

**只读查询**（`POST /dataway/query`）经 `internal/guard`，拒绝原因编码化：

| code | 触发条件 |
|---|---|
| `EMPTY_SQL` | 空语句 |
| `MULTI_STATEMENT` | 出现多个分号（防 `SELECT 1; DROP ...`） |
| `COMMENT_NOT_ALLOWED` | 出现 `--` 或 `/*`（防注释截断审计） |
| `ONLY_SELECT` | 不以 `SELECT` 开头 |
| `DML_NOT_ALLOWED` | 语句内出现写/DDL 关键字（含 CTE 夹带） |
| `TABLE_NOT_ALLOWED` | 引用白名单外的表 |
| `DATASOURCE_NOT_ALLOWED` | 数据源名未在服务端登记 |
| `TEMPLATE_NOT_FOUND` | 写入模板未登记（写入路径） |
| `PARAM_MISMATCH` | 模板要求的参数个数与实际不符（写入路径） |

判定顺序固定：同一句 SQL 只报第一个命中的 code，调用方不需要猜优先级。

**写入**（`POST /dataway/execute`）只认模板 ID：模板在 `internal/config/dataway.yaml` 登记，
参数个数必须匹配。模板在**启动时**逐条校验（单条 `INSERT/UPDATE/DELETE`、不含分号与注释），
任何一条不合规都不启动——配置退化要在启动时红，不要在第一次业务调用时红。

### 占位符改写

契约里的 SQL 用 JDBC 风格的 `?`，PostgreSQL 只认 `$n`，改写发生在网关内部
（`internal/guard/placeholders.go`）。改写跳过单引号字符串、双引号标识符与 `''` / `""` 转义，
`WHERE domain = 'a?b'` 里的问号是数据不是占位符。参数个数与改写结果来自同一遍扫描，
不会出现"数出 3 个、只替换 2 个"的错位。

## 配置

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SERVER_PORT` | `8090` | 监听端口 |
| `DATAWAY_CONFIG` | 内嵌 | 外部 YAML 路径，**整份替换**内嵌默认值（不合并） |
| `DATAWAY_DB_DSN` | 内嵌演示值 | 完整连接串；优先于分量拼装 |
| `DB_HOST` / `DB_PORT` / `DB_NAME` / `DB_USER` / `DB_PASSWORD` / `DB_SSLMODE` | — | 分量拼装 DSN，供 K8s 复用 `rcp-db-credential` Secret |
| `DATAWAY_DATASOURCE` | `main` | 客户端请求里允许出现的数据源名 |

表白名单与写入模板只存在于 `internal/config/dataway.yaml`（随二进制内嵌）：客户端只能引用
模板 ID，不能传连接串或自定义写入语句。

## 构建与验证

```bash
go vet ./... && go test ./... && go build ./...
python ../rcp-contracts/tools/contract_check.py --repo . \
  --contract ../rcp-contracts/contracts/openapi/dataway.yaml
docker build -t rcp-dataway:local .
```

单测覆盖：全部拒绝码（含 CTE 夹带、schema 前缀、白名单外的表）、占位符改写（字符串内问号、
转义引号、`CAST(? AS TIMESTAMP)`）、模板启动校验、参数个数错配、`maxRows` 上下界、
被拒绝的请求不触达数据库层、健康检查随数据库状态翻转。
数据库层用 `store.DB` 接口的 fake 实现，不需要真实 PostgreSQL；真实库的连接与查询形态
未在本仓单测中覆盖。

## 冒烟

```bash
go run . &
curl --noproxy '*' -X POST localhost:8090/dataway/execute -H 'Content-Type: application/json' \
  -d '{"datasource":"main","templateId":"verdict.insert","params":["job:1","INTERCEPT",80]}'
curl --noproxy '*' -X POST localhost:8090/dataway/query -H 'Content-Type: application/json' \
  -d '{"datasource":"main","sql":"SELECT event_id, decision FROM patrol_verdict"}'
```

## 已知边界

守卫是**规则集合**而不是 SQL 解析器：动态拼接、存储过程副作用、函数调用里的隐式写入都不在
本层拦截范围内。真正的兜底是数据源账号只读 + 模板写权限，守卫负责把常见越权路径挡在应用层
并在审计里留痕。这条边界不隐藏——写在这里，是因为把它说成"完全防注入"才是真风险。

其他已知边界：

- 改写只处理 `?`，不重排已存在的 `$n`。客户端混用 `?` 与 `$n` 时以 PostgreSQL 的绑定结果为准，
  不构成越权（仍受白名单与只读约束），但属于未定义的用法。
- `maxRows` 在读取端截断，不向调用方的 SQL 追加 `LIMIT`：网关只转发判断过的语句。
- 数据库错误细节（表名、谓词）只进服务端日志，不回给调用方。

## 从 Java 切到 Go 的破坏性变更

| 变更 | 影响 | 替代 |
|---|---|---|
| 构建 Maven → Go modules | `mvn verify` 不再适用 | `go vet/test/build`，CI 走 `go-service.yml` |
| 产物 jar → 静态二进制 | 镜像与启动命令变化 | 见 `Dockerfile`（alpine + 非 root） |
| 配置 `application.yml` → 内嵌 YAML + 环境变量 | `SPRING_DATASOURCE_*` 失效 | `DATAWAY_DB_DSN` 或 `DB_*` 分量 |
| 探活 `/actuator/health/readiness`、`/liveness` → `/actuator/health` | K8s 探针路径需改 | 单一探活，与 `rcp-iam` 一致 |
| 不再向 Nacos 注册 | 服务发现由 k8s DNS 承担（`fallback` 本来就是 `k8s-dns`） | 按 Service 名解析，清单不写上游 URL（RL-007） |
| 数据库驱动 PostgreSQL（pgx） | 只支持 PostgreSQL | 平台已全量 PostgreSQL 16 |
