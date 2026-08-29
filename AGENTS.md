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
  ├── database/  → 数据库初始化与连接管理
  ├── jwt/       → JWT token 生成与解析
  └── logger/    → Zap 日志初始化
sql/             → 数据库初始化脚本
```

### 核心架构模式

* **分层架构**：严格四层分离。handler 只做 HTTP 协议转换，不写业务逻辑；service 编排业务流程；repository 只做数据存取；model 定义数据结构

* **依赖注入**：当前通过函数参数传递依赖（轻量级），未使用 DI 框架

* **中间件链**：Recovery → Logger → CORS → Auth（按需），在 `main.go` 中统一注册

* **认证**：JWT Bearer Token，`middleware/auth.go` 解析并注入用户上下文

* **错误处理**：`pkg/errcode` 定义五位字符串业务码，handler 统一通过 `errcode.Success`/`errcode.Error` 返回 `{code: String, error, data}` 信封，HTTP 恒为 200（见 `docs/adr/0001-response-envelope-and-auth-model.md`）

* **数据库**：SQLite + GORM AutoMigrate，启动时自动建表/更新表结构

### API 路由结构

```
/health                    → 健康检查（无需认证）
/api/auth/send-code        → 按邮箱发注册/验证码
/api/auth/register         → 注册（邮箱+验证码+密码），激活建号，不发 token
/api/auth/login            → 邮箱+密码登录，返回双 token
/api/auth/refresh          → 刷新 token（轮换）
/api/auth/logout           → 登出（需认证）
/api/auth/forgot-password/send-code → 忘记密码发验证码（邮箱）
/api/auth/forgot-password/reset   → 验证码重置密码（邮箱）
/api/users/me              → 获取/更新当前用户信息（需认证）
/api/users/me/password     → 已登录修改密码（需认证）
/api/comments              → 评论列表（公开）/ 创建评论（需认证）
/api/comments/my           → 我的评论（需认证）
/api/comments/:id          → 删除评论（需认证）
/api/logs                  → 操作日志（需认证）
/api/logs/my               → 我的操作日志（需认证）
```

> **注册/账号模型**：`email` 为登录主标识（唯一必填），`uid` 为主键，`username`
> 非空但可重复（注册时自动生成，可后改）。详见 [ADR-0002](docs/adr/0002-email-based-registration-and-account-model.md)。

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

* handler 测试使用 `httptest` + Gin 测试模式，通过 `test_helper.go` 提供公共测试工具

* service 测试直接调用业务函数，mock repository 层

* 运行测试：`go test ./...`；提交前必须通过全量测试

* 覆盖率目标：model > 90%、pkg > 85%、service > 80%、handler > 75%

* 详细测试文档见 [TESTING.md](TESTING.md)

### 添加新接口的标准流程

1. `model/` — 定义数据模型和请求/响应结构体
2. `repository/` — 实现数据访问方法
3. `service/` — 实现业务逻辑
4. `handler/` — 实现 HTTP 处理器
5. `main.go` — 注册路由
6. 编写对应测试

## 配置说明

配置文件 `config.yaml`：

```yaml
server:
  port: 9090            # 服务端口
  mode: debug           # 运行模式: debug, release, test

database:
  path: ebook.db        # SQLite 数据库文件路径

jwt:
  secret: your-secret-key-change-this  # JWT 密钥（部署时必须修改）
  expire_min: 120                      # access token 过期时间（分钟）

smtp:
  host: smtp.example.com   # SMTP 服务器
  port: 465                # 465 走 TLS；587 走 STARTTLS
  username: no-reply@example.com
  password: your-smtp-password
  from: no-reply@example.com
  insecure: false          # 开发环境可设为 true 关闭 TLS 校验
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

