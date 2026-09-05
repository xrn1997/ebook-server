# 仓库指南（Repository Guidelines）

本文件是本仓库统一的 Agent 协作与贡献指南，供 Codex、Claude Code 等 Agent 与人工贡献者共用。

## 项目概述

电子书阅读器后端服务，Go 语言开发，Gin 框架，分层架构（handler → service → repository → model）。SQLite 单文件数据库，零配置启动。与 [android-ebook](https://github.com/xrn1997/android-ebook) Android 客户端配套使用。

本项目处于**开发阶段**，API 接口和功能仍在迭代中。

## 常用命令

```bash
# 安装依赖
go mod tidy

# 运行项目
go run main.go

# 构建二进制
go build -o ebook-server .

# 运行所有测试
go test ./...

# 运行测试（详细输出）
go test -v ./...

# 运行指定包的测试
go test -v ./service/...
go test -v ./handler/...

# 运行指定测试函数
go test -v -run TestLogin ./service/...

# 测试覆盖率
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# 代码检查（需安装 golangci-lint）
golangci-lint run

# Makefile 快捷命令
make build       # 构建
make run         # 运行
make clean       # 清理
make docker      # Docker 构建
make docs        # 重新生成 API 文档（swag init，产物入 backend/docs/）
```

## 项目架构

依赖方向：**handler → service → repository → model**（model 为基础层，无外部依赖）。

```
main.go          → 程序入口，路由注册，依赖组装
config/          → 配置管理（Viper 读取 config.yaml）
handler/         → HTTP 处理器（请求解析、参数校验、响应封装）
service/         → 业务逻辑（事务编排、业务规则）
repository/      → 数据访问层（GORM 查询，数据库交互）
model/           → 数据模型（GORM 实体、请求/响应结构体、错误定义）
middleware/      → Gin 中间件（JWT 认证、CORS、日志、panic 恢复）
pkg/             → 公共组件
  ├── code/      → 内存验证码存储与校验
  ├── database/  → 数据库初始化与连接管理
  ├── errcode/   → 五位字符串业务码与统一响应 helper
  ├── jwt/       → JWT token 生成与解析
  ├── logger/    → Zap 日志初始化
  ├── mail/      → SMTP 邮件发送
  └── ratelimit/ → 内存固定窗口限流器
sql/             → 数据库初始化脚本（MySQL 参考，实际用 SQLite + GORM AutoMigrate）
```

### 核心架构模式

* **分层架构**：严格四层分离。handler 只做 HTTP 协议转换，不写业务逻辑；service 编排业务流程；repository 只做数据存取；model 定义数据结构

* **依赖注入**：service 层通过定义在 `service/ports.go` 的 Store 接口（consumer-defined）访问数据，repository 是满足接口的 gorm adapter；依赖由构造函数注入，`main.go` 是唯一装配点。详见 [ADR-0007](docs/adr/0007-consumer-defined-repository-interfaces.md)

* **中间件链**：Recovery → Logger → CORS → Auth（按需），在 `main.go` 中统一注册

* **认证**：JWT Bearer Token，`middleware/auth.go` 解析并注入用户上下文

* **错误处理**：`pkg/errcode` 定义五位字符串业务码，handler 统一通过 `errcode.Success`/`errcode.Error` 返回 `{code: String, error, data}` 信封，HTTP 恒为 200（见 `docs/adr/0001-response-envelope-and-auth-model.md`）

* **数据库**：SQLite + GORM AutoMigrate，启动时自动建表/更新表结构

### API 路由结构

```
/health                    → 健康检查（无需认证）
/api-docs                  → Swagger UI + OpenAPI spec（**公开端口默认关闭**，由 `api_docs.enabled` 控制；后台端口 9091 始终提供，供本机查阅）
/api/auth/send-code        → 按邮箱发注册/验证码
/api/auth/register         → 注册（邮箱+验证码+密码），激活建号，不发 token
/api/auth/login            → 邮箱+密码登录，返回双 token
/api/auth/refresh          → 刷新 token（轮换）
/api/auth/logout           → 登出（需认证）
/api/auth/forgot-password/send-code → 忘记密码发验证码（邮箱）
/api/auth/forgot-password/reset   → 验证码重置密码（邮箱）
/api/users/me              → 获取/更新当前用户信息（需认证）
/api/users/me/password     → 已登录修改密码（需认证）
/api/users/me/data         → 导出我的数据（用户资料+本人评论，需认证）
/api/users/me/deletion/send-code → 发注销验证码到当前账号邮箱（需认证）
/api/users/me/deletion     → 注销账号（验证码确认，匿名化并返回数据副本，需认证）
/api/comments              → 评论列表（公开，chapter_url 支持多个返回并集；book_name 可单独或配合过滤）/ 创建评论（需认证，支持章节字段）
/api/comments/migrate-key  → 迁移评论聚合键（需认证，旧键→新键，仅本人，同键 A0305）
/api/comments/my           → 我的评论（需认证）
/api/comments/:id          → 删除评论（需认证，仅本人，非本人 A0303）
/api/uploads/avatar        → 头像上传（需认证，multipart，返回绝对 URL）
/uploads/*                 → 上传文件静态访问（公开，仅头像等公开资源）
/api/logs                  → 操作日志（需认证）
/api/logs/my               → 我的操作日志（需认证）
```

> **后台管理系统（`/admin*`）**：与公开 API 拆分为**独立 Gin 引擎 + 独立监听地址**（网络层隔离）。
> 公开 API 监听 `0.0.0.0:<server.port>`（默认 9090）；后台默认仅监听
> `127.0.0.1:<admin.listen_port>`（默认 9091），公网物理上无法连接后台。
> 后台使用独立 JWT 鉴权（`admin.jwt_secret`，与用户 token 互不可用），登录接口按
> 来源 IP 限流 5 次/分钟（超限返回 `A0241`）。远程管理请走 SSH 隧道/VPN，
> 不要直接开放后台端口。需局域网访问时改 `admin.listen_addr` 为内网 IP。
>
> **评论与头像契约（ADR-0011）**：评论支持章节归属——`chapter_url`（书源章节 URL，
> 聚合键）/`chapter_name`/`book_name` 为冗余快照，可选、不校验格式仅限长；
> 空章节 = 书籍级评论。评论响应用独立视图（user 只含 uid/username/nickname/avatar，
> add_time 固定上海时区）。头像两步提交：`POST /api/uploads/avatar` 拿 URL →
> `PUT /api/users/me` 更新；更换头像自动删除本服务旧文件（`upload.dir` 默认 uploads）。

> **注册/账号模型**：`email` 为登录主标识（唯一必填）且**不可变**，`uid` 为主键，`username`
> 非空但可重复（注册时自动生成，可后改）。改资料时若传了不同的 `email` 返回 `A0113`。
> 详见 [ADR-0002](docs/adr/0002-email-based-registration-and-account-model.md)
> 与 [ADR-0004](docs/adr/0004-login-identifier-immutable.md)。
>
> **账号注销**：采用匿名化而非删除——改写 `email` 为占位值、清空密码与头像，
> **不设置** **`DeletedAt`**（软删会让 GORM 的 `Preload` 过滤掉评论作者）。
> 公开评论与操作日志保留。详见 [ADR-0005](docs/adr/0005-account-deletion-by-anonymization.md)。

## 技术栈

* **语言**: Go 1.22

* **Web 框架**: Gin

* **ORM**: GORM + SQLite（glebarez/sqlite，纯 Go 驱动，无需 CGO）

* **认证**: JWT（golang-jwt/jwt/v5）

* **配置**: Viper（支持 YAML）

* **日志**: Zap（结构化日志）

## 构建约定

* 遵循 Go 标准项目布局，`cmd/` 暂未使用（单入口 `main.go`）

* 依赖管理使用 Go Modules（`go.mod` / `go.sum`），不要手动修改

* 新增依赖使用 `go get <package>@latest`，提交前运行 `go mod tidy` 清理

* 配置文件 `config.yaml` 不含真实密钥（仅占位符），可安全提交

* `.env` 文件已在 `.gitignore` 中排除，禁止提交真实密钥

* **注释要求**：导出函数、类型和重要逻辑分支必须有注释，说明"是什么、为什么"

* **文档同步**：代码与文档（本文件 / README.md / TESTING.md）必须保持一致——任何改动在提交前同步更新相关文档

* 不要引入 `golint` 警告，提交代码应保持警告清洁

* 数据库文件 `*.db` 已在 `.gitignore` 中排除，禁止提交

## 测试约定

* 测试文件与被测文件同目录，命名 `<subject>_test.go`

* 测试函数命名 `TestXxx`（Go 标准）

* handler 测试使用 `httptest` + Gin 测试模式，通过 `test_helper_test.go` 提供公共测试工具

* service 测试直接调用业务函数，数据访问用 SQLite `:memory:` 独立实例（`pkg/testdb.Open`）——不写 mock，测试跑的是真 SQL 语义（ADR-0007）

* 运行测试：`go test ./...`；提交前必须通过全量测试

* 覆盖率目标：model > 90%、pkg > 85%、service > 80%、handler > 75%

* 详细测试文档见 [TESTING.md](TESTING.md)

### 添加新接口的标准流程

1. `model/` — 定义数据模型和请求/响应结构体
2. `service/ports.go` — 如需新的数据访问能力，先在对应 Store 接口上扩方法（接口由消费方定义）
3. `repository/` — 实现数据访问方法
4. `service/` — 实现业务逻辑
5. `handler/` — 实现 HTTP 处理器
6. `main.go` — 装配依赖并注册路由
7. 编写对应测试

## 配置说明

配置文件 `config.yaml`：

```yaml
server:
  port: 9090            # 服务端口
  mode: debug           # 运行模式: debug, release, test

database:
  path: ebook.db        # SQLite 数据库文件路径

api_docs:
  enabled: false        # 公开 API 端口是否提供 Swagger 文档（默认关，防接口清单泄露；联调时改 true）

jwt:
  secret: your-secret-key-change-this  # JWT 密钥（部署时必须修改）
  expire_min: 120                      # access token 过期时间（分钟）

admin:
  username: admin            # 后台账号
  password: change-me        # 后台密码（真实值放 .env 的 ADMIN_PASSWORD）
  jwt_secret: your-admin-secret-change-this  # 管理端 JWT 密钥（.env 的 ADMIN_JWT_SECRET 覆盖）
  expire_min: 60             # 管理端 token 有效期（分钟）
  listen_addr: 127.0.0.1     # 后台监听地址：默认仅本机（公网不可达）；局域网访问改内网 IP
  listen_port: 9091          # 后台监听端口（与公开 API 的 9090 分离）

smtp:
  host: smtp.example.com   # SMTP 服务器
  port: 465                # 465 走 TLS；587 走 STARTTLS
  username: no-reply@example.com
  password: your-smtp-password
  from: no-reply@example.com
  insecure: false          # 开发环境可设为 true 关闭 TLS 校验

upload:
  dir: uploads             # 上传文件根目录（头像存 <dir>/avatar/，经 /uploads/* 公开）
```

> **SMTP 未配置**时，找回密码验证码会退回写入日志（`pkg/mail`），便于本地联调。
> **敏感项走** **`.env`**：`smtp.password` 用 `.env` 的 `SMTP_PASSWORD` 覆盖（`config.yaml` 留占位符），
> `JWT_SECRET` 同理；`.env` 已在 `.gitignore` 排除。

## 部署

### Docker

```bash
docker build -t ebook-server .
docker run -p 9090:9090 ebook-server
```

### 直接运行

```bash
go build -o ebook-server .
./ebook-server
```

## 已知问题

* **验证码存储为内存 map**：`pkg/code` 单进程可用，多实例部署需迁移至 Redis/共享存储（ADR-0001）

* **refresh token 无效 session 清理**：登出/改密会作废用户全部 refresh token，过期记录由数据库接管，暂无定时清理任务

* **SQLite 并发限制**：SQLite 写入串行化，高并发场景需评估是否迁移至 MySQL/PostgreSQL

* **数据库迁移**：使用 GORM AutoMigrate，生产环境需谨慎评估表结构变更

* **access token 无法主动作废**：改密/重置/注销只删除 `refresh_tokens` 记录，而 access token 是无状态
  JWT、校验时不查库，因此旧 access token 在其剩余 2 小时有效期内仍可调通 API。ADR-0001 §5 承诺的
  "旧 access/refresh 一律失效"仅兑现了 refresh 一半。彻底解决需引入 token 黑名单，属基础设施依赖，
  当前不做——详见 `docs/adr/0005-account-deletion-by-anonymization.md`

* **~~操作日志无人写入~~**（已解决）：曾因 `service/log.go`、`repository/log.go` 的 `Create`
  无生产调用方，`GET /api/logs*` 恒返回空。现由 `middleware/operationlog.go` 把每次客户端请求
  写入 `operation_logs`（刻意**不写** `model.OperationLog.RequestBody`，防登录明文密码）；
  `/admin*` 为后台自身流量，不入库。后台管理端经 `GET /admin/api/logs` 查看审计，
  客户端经 `GET /api/logs/my` 查看自己的请求。

* **账号注销流程不包事务（有意为之）**：`AccountService.Delete` 依次执行「删 refresh token →
  匿名化账号」两次写，中途失败只会留下「token 已删但账号未匿名化」的状态——用户重新登录
  即可恢复并重试注销，无锁死、无数据丢失；而真正危险的问题（access token 匿名化后仍有效）
  是无状态 JWT 的固有缺陷，事务治不了（见上条）。跨 repo 事务需引入 TxRunner seam，
  为一个可自愈的瞬态加这层间接不划算。审查者不要再标记此条——加事务前先推翻这里的推理

* **`ErrMailSendFailed`** **无产生方**：`pkg/mail` 的发送错误原样向上返回，handler 中
  `err == model.ErrMailSendFailed` 的分支永远不会命中，SMTP 失败实际落 `C0500`。
  若要让 `C0503` 生效，需在 service 层把邮件错误包装为 `ErrMailSendFailed`

## Agent 实战建议

* 修改 handler 前，先确认对应的 service 和 repository 层是否需要变更

* 涉及认证改动时，确认 middleware/auth.go 的 JWT 解析逻辑不受影响

* 新增 API 接口需同步更新 README.md 的 API 文档

* 数据库 model 变更后，确认 AutoMigrate 能正确处理（尤其字段类型变更和索引）

* 路径中使用正斜杠 `/`，Windows 下 Git Bash 兼容

* 涉及响应信封或认证模型的改动，先查阅 `docs/adr/0001-response-envelope-and-auth-model.md` 确认设计决策

## 提交规范

本仓库遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
<type>(<scope>): <description>

[body]

[footer]
```

### Type（标准类型，不得自定义）

| Type       | 说明                                   |
| ---------- | ------------------------------------ |
| `feat`     | 新功能                                  |
| `fix`      | Bug 修复                               |
| `build`    | 构建系统 / 依赖变更（Go Modules、Dockerfile 等） |
| `refactor` | 重构（不改变外部行为）                          |
| `docs`     | 文档变更                                 |
| `test`     | 测试变更                                 |
| `perf`     | 性能优化                                 |
| `chore`    | 杂务（CI 配置、脚本、工具链等）                    |
| `revert`   | 回退提交                                 |

### Scope

Scope 直接使用模块目录名：

* **业务模块**：`handler` / `service` / `repository` / `model`

* **基础设施**：`middleware` / `pkg` / `config`

* **横切关注点**：`docs` / `sql`

* **可省略**：改动仅涉及根目录文件（如 `.gitignore`、`AGENTS.md`）或与 scope 无关的杂务时，可省略 scope

### Description

* 使用中文，不超过 72 个字符

* **动词前置**：以动词开头（新增…、修复…、重构…、升级…），禁止名词性短语（如"关于…的修改"、"xxx 的变更"）

* **英文缩写保持原样**：JWT、GORM、Gin、API 等专有名词保持原始大小写，不强行翻译

* **禁止模糊表述**：不允许"更新了 xxx"、"优化了一下"等无信息量描述

* **禁止嵌入 issue 编号**：编号放在 footer（`Closes #123`），不要写在 description 里

* **不加句号**

### Body

* **何时写**：单行 description 不足以解释动机或影响时，用 body 补充 why / how

* **格式**：与 header 之间空一行；**正文优先用列表分条目**（每个条目一个改动点，条目按行换行、续行缩进两空格），每行不超过 100 字符，禁止一整段超长单行；条目组之间空一行

* **语言**：与 description 一致（中文）

### Footer

* **Breaking Change**：在 footer 中声明 `BREAKING CHANGE: <描述>`，或在 type 后加 `!`（如 `feat!: ...`）

* **关联 Issue**：`Closes #123` / `Refs #456`

### 语义版本映射

提交类型与版本号 bump 的对应关系（Conventional Commits 核心价值，用于自动化版本发布）：

| type / footer                                                                | 版本 bump       | 示例        |
| ---------------------------------------------------------------------------- | ------------- | --------- |
| `fix`                                                                        | PATCH (0.0.x) | 修复崩溃      |
| `feat`                                                                       | MINOR (0.x.0) | 新增功能      |
| `BREAKING CHANGE` / `!`                                                      | MAJOR (x.0.0) | API 不兼容变更 |
| `build` / `chore` / `docs` / `test` / `refactor` / `perf`（无 BREAKING CHANGE） | **不 bump**    | 版本号不变     |

任何 type 都可以携带 `BREAKING CHANGE`（不限于 `feat`/`fix`），如 `refactor!: 删除废弃 API` 触发 MAJOR。

### Revert 格式

Revert 有特殊格式约定，`description` 为被回退提交的完整 header，body 中必须包含 `This reverts commit <hash>`：

```
revert: feat(handler): 新增书签收藏接口

This reverts commit abc1234.
原因: 数据库迁移脚本有误，待修复后重新合入。
```

### 示例

常规提交：

```
feat(handler): 新增书签收藏接口               → MINOR bump (0.x.0)

引入 /api/bookmarks 路由，支持登录用户
对章节进行收藏与取消收藏操作。

Closes #42
```

```
fix(service): 修复评论删除未校验用户归属       → PATCH bump (0.0.x)

删除评论时增加 ownership 校验，
防止用户删除他人评论。
```

```
build: 升级 Gin 到 v1.10.1                    → 不 bump
```

Breaking change 提交（两种写法等价）：

```
refactor!: 重构 JWT token 过期时间配置         → MAJOR bump (x.0.0)

将 token 过期时间从硬编码改为 config.yaml 配置，
旧配置文件需新增 expire_hour 字段。

BREAKING CHANGE: config.yaml 必须包含 jwt.expire_hour 字段
```

### 提交前验证

* 运行 `go test ./...`，确保全量测试通过

* 运行 `go mod tidy`，确保依赖干净

* 涉及 API 变更时，同步更新 README.md

## Agent skills

### Issue tracker

GitHub Issues — 使用 `gh` CLI 操作。See `docs/agents/issue-tracker.md`.

### Domain docs

单仓布局（single-context）—— `CONTEXT.md` + `docs/adr/` 位于仓库根目录。See `docs/agents/domain.md`.

**领域文档**：

* **架构决策记录**：`docs/adr/`。重大决策（难回退 / 无上下文令人惊讶 / 有真实权衡）必须记录

* 本项目为单上下文仓库，无 CONTEXT-MAP.md

## Pull Request 指南

* 描述变更内容与动机，并关联对应 Issue

* 列出已运行的测试（如 `go test -v ./service/...`）

* 每个 PR 聚焦单一变更，避免夹带无关修改

