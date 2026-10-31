# ebook-server — Agent 工作须知

Go 后端 + 内嵌 web 后台；公开 API 与后台 API 是两个独立监听表面。本文件只放每次会话必须看到的规则；架构细节、命令全集、决策理由各有自己的家。

## 先读哪份

| 你要回答的问题 | 读 |
| --- | --- |
| 用户怎么装、怎么用、API/配置是什么 | `README.md` |
| 这个词在本仓指什么 | `CONTEXT.md` |
| 系统分成哪几块、某条语义归谁 | `docs/architecture.md` |
| 怎么构建、运行、测试、发版 | `docs/development.md` |
| 当初为什么这么定、否掉了什么 | `docs/adr/`（索引 `docs/adr/README.md`） |
| 文档该放哪、怎么写、何时归档 | `docs/AGENTS.md` |
| 怎么提交、PR、发布边界 | `CONTRIBUTING.md` |
| 测试策略与覆盖率口径 | `TESTING.md` |

一条事实只住一个家；代码与测试是最终真相。文档与代码冲突时改文档。

## 硬约束

- 依赖方向 `handler → service → repository → model`；gorm 类型不出 repository；装配只在 `backend/main.go`。
- 路由表住 `backend/router/router.go`；新路由必须登记并过 `backend/router/router_test.go`；需要登录的路由挂在路由组上。
- 响应信封与业务码只走 `backend/pkg/errcode`；成功码常量是 `model.SuccessCode`（数据层比较用），errcode 以别名引用。绑定/校验错误只走 `errcode.RespondBindError`；未登记错误不回原始文案，只写日志。
- 配置字段写在 `backend/config/config.go` 的 `mapstructure` 标签里；默认值、环境变量覆盖与校验规则只住 `backend/config/schema.go`；`config.Load` 拒绝空密钥、非法端口、非法运行模式/有效期与通配后台监听。
- 评论聚合键 `comment_key` 服务端只存不解释；创建评论缺键回 `A0400`；`chapter_url`/`book_name` 只作兼容/展示。作者删与后台治理删共用 `CommentService.Delete(id, scope)`，以 `RowsAffected` 为成功判据。
- 账号注销是**匿名化**而非删除：清空可识别信息、不设 `DeletedAt`，公开内容保留；导出副本在清空前构建。
- 操作审计只挂 `/api` 组；`/admin*` 与 `/health` 不入库；跨用户审计读取只在后台 `/admin/api/logs`。
- 后台是独立 Gin 引擎 + 独立 JWT，默认只监听 `127.0.0.1`；远程管理走 SSH 隧道/VPN。
- 管理员凭据不进前端；后台 token 存浏览器 `localStorage`，只用于同源 `/admin/api` 调用，鉴权失败立即清除并回登录页。
- 头像文件与资料列同生命周期；换头像删旧文件前按文件名确认没有其他账号引用；注销时尽力删除。
- service/repository 的请求作用域方法第一参数是 `context.Context`；handler 传 `c.Request.Context()`，repository 使用 `WithContext`。
- schema 变更必须加 `backend/migrations` 迁移文件；生产启动走 `database.Migrate`，测试库才用 AutoMigrate。
- SQLite pragmas（WAL/busy_timeout/foreign_keys）只住 `backend/pkg/database`；Logger 不读 request body，request id 由 `middleware.RequestID` 写入。

## 必过门

- 改 Go：`cd backend && go test ./...`（已包含 `docscheck` 与 `archtest`）
- 改 frontend：`make frontend-test`（`vue-tsc` + `vitest`）
- 改 Swagger 注解：`make docs`
- 提交前：`go vet ./...` 与 `go test -race ./...`（或 `make lint` / `make test-race`）；`git diff` 检查只包含预期文件；不提交 `.env`、`*.db`、构建产物

## 环境坑

- 服务必须从仓库根启动：`config.yaml`、`ebook.db`、`uploads` 都按工作目录解析；`backend/` 下 `go run main.go` 读不到配置。用 `make run`。
- `build/`、`frontend/dist/` 与 `backend/internal/admin/web/` 的镜像产物不入库；`make clean` 会清掉前端产物与 embed 镜像。
- Windows 独立服务端构建依赖 MinGW-w64 的 `windres`；非 Windows 主机交叉构建同样需要。

## 提交与协作

提交规范、PR 约定与提交前检查见 `CONTRIBUTING.md`。Issue tracker 与 domain docs 见 `docs/agents/`。

## Agent skills

- Issue tracker：GitHub Issues，使用 `gh` CLI。见 `docs/agents/issue-tracker.md`。
- Domain docs：单上下文仓库，`CONTEXT.md` + `docs/adr/`。见 `docs/agents/domain.md`。
