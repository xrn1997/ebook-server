# 操作审计的接入边界与有界捕获

Status: accepted

操作日志记录「客户端业务请求」。此前这条规则写在审计中间件里：判断路径是否以 `/admin`
开头、是否等于 `/health`。中间件挂在公开引擎的根上，所以它确实看得到后台前缀（虽然后台
其实跑在另一个引擎上，这个分支永远走不到）——规则与挂载点各说各话，读完代码也说不清
「到底哪些流量会入库」。同时响应体被完整复制进内存：评论列表这类成功响应可以有几十 KB，
审计为了读开头的 `{code,error}` 却把整份响应又存了一遍。

## 决策

### 1. 接入边界由路由组决定，审计规则由日志模块决定

* 公开引擎的中间件链收敛为 RequestID → Recovery → Logger → CORS；审计中间件只挂在
  `/api` 路由组上。`/health`、`/api-docs`、`/uploads/avatar` 从物理上不经过它。
* 中间件不做路径过滤，只把事实（方法、路径、IP、UA、状态码、当前用户、信封业务码）
  交给 `OperationRecorder`。
* `LogService.Record` 是「什么算一次操作」的唯一实现：`/admin*` 与 `/health` 不写库。
  接入方式变了（换路由组、加表面），规则仍只有这一处。

### 2. 响应捕获有界且只认 JSON

`recordWriter` 在第一次写入时检查 `Content-Type`：非 JSON 响应没有信封，不占缓冲；
JSON 最多保留 4 KiB（`maxEnvelopeBytes`）。截断只作用于审计缓冲，真实响应完整转发
（`TestRecordWriter_BoundsJSONCapture` 同时断言这两件事）。

超大成功响应被截断后解析不出业务码，`error_code` 留空；后台「只看失败」的筛选条件是
「非空且非 00000」（`repository.FindBy`），空码不会被误判为失败。

### 3. 写入仍是同步的

审计写入失败只告警、不阻断请求（`zap.Warn`）。不引入异步队列：SQLite 单机部署下，
一次 INSERT 的延迟远小于请求本身；异步化会带来「进程退出时丢审计」与「队列无界」
两个新问题，而当前没有吞吐证据支持这个代价。RequestBody 仍然刻意不记录（防登录明文密码）。

写入使用 `context.WithoutCancel` + 短超时：客户端断开后请求上下文已取消，直接拿它写库会让
INSERT 必然失败，而断开的请求恰恰最值得留下记录。`Path`、`User-Agent`、`ErrorMessage`
落库前按字节上限截断（UTF-8 完整），避免匿名请求用 MB 级请求行/头制造写放大。

## 后果

* 路由表调整（例如把某个端点移出 `/api`）会自然改变审计范围，不需要同步修改中间件。
* 后台管理端（`/admin/api/logs`）仍是跨用户读取审计的唯一入口；客户端只能读自己的
  （`/api/logs/my`）。审计范围收窄不影响这两条读路径。
* 4 KiB 上限意味着「成功但超大」的响应不带业务码；按错误码统计失败数不受影响。
* 日志模块的过滤规则是域规则，可被直接单测（`TestLogService_Record_FiltersNonClientTraffic`），
  不再依赖「中间件挂在哪里」这种装配事实来验证。

测试：`service/log_test.go`（过滤、字段落库与客户端字段截断）、
`middleware/operationlog_test.go`（只转发、有界捕获、非 JSON 跳过、写入失败不阻断、
请求取消后审计上下文仍可用、登录身份随事实一起交出、未登录不编造身份、panic 时记录为 C0500）、
`router/router_test.go`（`/api` 审计已挂载、`/health` 不经过）。

锚点：`backend/middleware/operationlog.go`；`backend/service/log.go`；`backend/router/router.go`；`backend/middleware/operationlog_test.go`。
