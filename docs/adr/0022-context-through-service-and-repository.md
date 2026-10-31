# context 贯穿 service 与 repository

Status: accepted

service 与 repository 的每个请求作用域方法第一个参数是 `context.Context`；handler 传 `c.Request.Context()`，repository 的每个 GORM 查询使用 `WithContext`。这样客户端断开、请求超时与上层取消能沿调用链传下去。

## 边界

- 纯计算、构造器、JWT 签发/解析、内存验证码存储不强制带 ctx。
- `Mailer` 与 `AvatarCleaner` 保持无 ctx：`net/smtp` 与 `os.Remove` 没有可用的取消路径；给它们加一个不会被使用的参数只会制造假接口。
- 审计写入是例外：它用 `context.WithoutCancel` + 短超时（ADR-0018）。记录的是已经发生的请求事实，不能因为客户端断开（请求 ctx 已取消）而丢审计。
- 测试直接调用 service/repository 时传 `context.Background()`。
- 不改对外 API 与响应信封；后台前端只经 `/admin/api` 调用，没有进程内契约要同步。

## 被否方案

- **只在 repository 加 ctx**：handler 与 service 之间的取消传不进去，长查询/慢服务仍会占着 goroutine；否决。
- **给所有 helper 一律加 ctx**：SMTP/文件删除没有取消语义，参数会变成装饰；否决。
- **在 handler 里新建 `context.Background()`**：会丢掉客户端断开信号；否决。

锚点：`backend/service/ports.go`；`backend/repository/user.go`；`backend/service/auth.go`；`backend/handler/auth.go`。
