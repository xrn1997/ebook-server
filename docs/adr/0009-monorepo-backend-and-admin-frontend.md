# 前后端 monorepo 重组：backend/ + frontend/，单 exe 内嵌 Vue 管理前端

将一个纯 Go 仓库重组为「Go 后端 + Vue 前端」的单一 monorepo：全部 Go 代码迁入 `backend/`，
新增 `frontend/`（Vue 3 + Vite）承载后台管理系统，构建时把前端产物灌进 `backend/internal/admin/web`
并以 `go:embed` 打进同一可执行文件——交付形态仍是单 exe。后台作为独立表面，与公开 API 分离鉴权。

## 现状

仓库是纯 Go 模块（go.mod 位于根目录）：`main.go` + `handler/ service/ repository/ model/ middleware/ pkg/`。
对外是认证/账号/评论/日志的 REST API。无任何前端、无管理界面。

新增需求：需要一个后台管理系统，功能含只读运维概览、用户管理、内容审核、数据统计/图表，
管理员使用，与普通用户的公开 API 严格区分。

## 动机

* **统计图表决定了前端是 JS**：图表库（ECharts 等）是前端组件，纯 Go 模板难以支撑。
  因此后台必然引入一套 Web 前端，仓库不再可能保持「纯 Go」。

* **职责清晰的两层归属**：纯 Go 与前端工具链（Node/npm）、依赖、构建方式完全不同，
  揉在同一个目录层级会互相污染。按 `backend/`、`frontend/` 顶层切分，各自可独立构建/CI/部署。

* **保持单 exe 的自托管交付**：repo 定位是开源自托管、开箱即用。后台前端构建产物是纯静态文件，
  用 `go:embed` 打进 Go 二进制，交付仍是单文件，不动现有部署方式。

* **后台不与公开 API 同权**：后台面向管理员，需独立鉴权，不能复用公开用户 JWT；
  放在 `internal/admin` 独立表面，复用现有 service/repository 但横向切干净，避免职责膨胀。

## 决策

### 1. monorepo 顶层结构

```
ebook-server/
├── backend/        # Go module（现全部 Go 代码迁入，go.mod/go.sum 一并迁移）
├── frontend/       # Vue 3 + Vite 后台前端（独立 npm 工具链，不入 Go module）
├── docs/ sql/ README.md AGENTS.md CONTEXT.md TESTING.md config.yaml
├── Makefile        # build = 前端 build → 灌 dist → go build(单 exe)
└── Dockerfile      # 改为 backend/ 多阶段构建
```

* Go 入口保持单入口 `backend/main.go`（沿用「cmd/ 暂不使用」的既有约定）。

* 公开 REST API 继续由 `backend/handler` 承载，路由在 `backend/main.go` 统一注册。

### 2. 后台独立表面（backend/internal/admin）

* `backend/internal/admin/`：管理端鉴权 + 薄 handler，复用 `backend/service`、`backend/repository` 的现有能力。

* **独立鉴权**：后台用独立凭证/token，不复用公开用户 JWT（ADR-0001 认证模型只覆盖公开面）。

* 路由独立前缀（如 `/admin/*`），与公开 API 分开。

### 3. go:embed 单 exe（embed 不能跨目录的解法）

`//go:embed` 引用路径不得含 `..`，故 `frontend/dist` 无法被 `backend` 直接 embed。解法：

* `frontend` 构建产物落在 `frontend/dist/`。

* `Makefile`（或 CI）把 `frontend/dist/*` **灌入** `backend/internal/admin/web/`。

* `backend/internal/admin` 内 `//go:embed all:web` 进二进制。

* 产物仍是单 exe：运行时由 Go 静态服务 `web/`，并挂到 `/admin` 前端路由。

### 4. 后台范围（四块）

只读运维概览（状态/健康/最近日志）、用户管理（列表/搜索/封禁）、内容审核（评论管理）、
数据统计/图表。以 `frontend/src/` 的页面与 `backend/internal/admin/` 的 handler 对应实现。

## 被拒方案

* **go.mod 留在仓库根、`frontend/`** **作子目录**：embed 最顺（`//go:embed frontend/dist` 合法），
  但「纯 Go vs 全栈」界限模糊，根目录会同时是 module 根与前端所在，所有权/CI 不清。

* **后台塞进公开 handler**：公开 API 与管理 API 同权同层，攻击面与权限完全混淆，
  违反「管理面独立鉴权」与「职责不过度膨胀」。

* **独立前端仓单独部署（不 embed）**：退回多产物/CORS/跨域部署，破坏单 exe 自托管定位。

## 下游影响

* 全部 Go 源文件与 go.mod 迁入 `backend/`，`main.go` 变 `backend/main.go`。

* `backend/internal/admin` 新增：管理鉴权、users/comments/stats handler、`web/`(go:embed)。

* `frontend/` 新增 Vue 3 + Vite 工程：`src/views/{overview,users,comments,stats}` + `api/` client。

* `Makefile`/`Dockerfile` 增加「前端 build → 灌 dist → go build」编排。

* `README.md`/`AGENTS.md`/`TESTING.md` 同步更新结构说明与命令。

* 提交规范 scopes 沿用模块目录名，`admin` 作为新业务 scope。

## 验收

1. 结构迁入 `backend/` 后 `go build ./...`、`go test -count=1 ./backend/...` 全绿，对外 REST API 行为不变。
2. `make build` 从零产出单文件 `ebook-server.exe`（内嵌 `frontend/dist/www` 静态资源与 index.html）。
3. 浏览器 `GET /admin` 返回内嵌前端首页；`backend/internal/admin` 的接口有独立鉴权，与公开 JWT 互不可用。
4. 公开 API（认证/账号/评论）在重组后请求/响应与迁移前完全一致。

