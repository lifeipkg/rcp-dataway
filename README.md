# rcp-dataway · 数据网关

业务服务访问数据库的**唯一出口**（红线 RL-005）。业务服务不持 JDBC 驱动、不写裸 SQL。

| 项 | 值 |
|---|---|
| 服务名 | `dataway` |
| 端口 | `8090` |
| 契约 | `rcp-contracts/contracts/openapi/dataway.yaml` |

## 两道闸

**只读查询**（`POST /dataway/query`）经 `SqlGuard`，拒绝原因编码化：

| code | 触发条件 |
|---|---|
| `EMPTY_SQL` | 空语句 |
| `MULTI_STATEMENT` | 出现多个分号（防 `SELECT 1; DROP ...`） |
| `COMMENT_NOT_ALLOWED` | 出现 `--` 或 `/*`（防注释截断审计） |
| `ONLY_SELECT` | 不以 `SELECT` 开头 |
| `DML_NOT_ALLOWED` | 语句内出现写/DDL 关键字（含 CTE 夹带） |
| `TABLE_NOT_ALLOWED` | 引用白名单外的表 |
| `DATASOURCE_NOT_ALLOWED` | 数据源名未在服务端登记 |

**写入**（`POST /dataway/execute`）只认模板 ID：模板在 `application.yml` 登记，参数个数必须匹配（`PARAM_MISMATCH`）。

## 已知边界

守卫是**规则集合**而不是 SQL 解析器：动态拼接、存储过程副作用、函数调用里的隐式写入都不在本层拦截范围内。真正的兜底是数据源账号只读 + 模板写权限，守卫负责把常见越权路径挡在应用层并在审计里留痕。这条边界不隐藏——写在这里，是因为把它说成"完全防注入"才是真风险。

## 构建与验证

```bash
mvn -B verify
python ../rcp-contracts/tools/contract_check.py \
  --repo . --contract ../rcp-contracts/contracts/openapi/dataway.yaml
```

## 冒烟

```bash
java -jar target/dataway-1.0.0.jar --server.port=8090 &
curl --noproxy '*' -X POST localhost:8090/dataway/execute -H 'Content-Type: application/json' \
  -d '{"datasource":"main","templateId":"verdict.insert","params":["job:1","INTERCEPT",80]}'
curl --noproxy '*' -X POST localhost:8090/dataway/query -H 'Content-Type: application/json' \
  -d '{"datasource":"main","sql":"SELECT event_id, decision FROM patrol_verdict"}'
```
