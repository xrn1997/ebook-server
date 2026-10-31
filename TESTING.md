# 测试文档

本文档描述了 ebook-server 项目的单元测试策略和实现。

## 测试概述

### 测试目标

- **代码覆盖率**: 按**逐包**计（`go test -count=1 -cover ./...` 每个包各一行），不把一棵子树合并成一个数字
- **关键模块覆盖率**: model > 90%, `pkg/` 下每个**有测试文件的**包 > 85%, service > 80%, handler > 75%
- **测试类型**: 单元测试、集成测试

### 测试框架

- Go 标准测试包 `testing`
- HTTP 测试 `net/http/httptest`
- 前端 Vitest + `@vue/test-utils`；不额外引入 Go 断言库

## 测试结构

测试文件与被测文件同目录，命名 `<subject>_test.go`；下表按**包**列（文件清单用
`find backend -name '*_test.go'` 现查，抄进文档一定会过期）。

| 包 | 钉住什么 |
|------|----------|
| `.`（main） | 装配层按配置选择邮件 adapter（SMTP / 日志） |
| `config` | 配置缺省值、`.env` 覆盖敏感项、无包级全局；**拒绝不安全配置启动**（空密钥任何模式、内置默认值 `release` 下）；**校验规则拒绝非法值**（`schema_test.go`） |
| `model` | 分页钳制、评论键边界与归属范围（`CommentKeyScope`）、删除范围（`CommentScope`）与后台固定视图 |
| `pkg/code` | 验证码存储：TTL、尝试次数、无 `Set` 后门 |
| `pkg/errcode` | 哨兵 → 业务码沿包装链查表、端点级 `Map` 覆盖；绑定/校验错误经 `RespondBindError` 翻成中文，并断言 validator 的英文原文不出现在响应里 |
| `pkg/httpx` | 请求取值：分页钳制与数字 ID 边界（详见下节） |
| `pkg/jwt` | `TokenBox` 签发/解析、算法固定、双 adapter 互不可用 |
| `pkg/ratelimit` | 固定窗口配额与窗口滚动 |
| `pkg/upload` | 头像校验/命名/删除/路径穿越 |
| `middleware` | 鉴权、CORS、panic 恢复、操作审计中间件只转发事实（路径过滤在 service 层）、有界 JSON 捕获 |
| `repository` | 真 SQL 语义（`:memory:`）+ 「查不到」在边界翻成领域哨兵 + 条件 DELETE 以 `RowsAffected` 判成功 + 凭据/资料的列级写各只碰自己的列 |
| `service` | 认证/账号/评论/验证码业务规则；资料与头像文件生命周期、密码/锁定/会话撤销 |
| `handler` | HTTP 协议转换与响应信封（经 `newTestApp` 装配整条链） |
| `router` | 路由表：登记清单与 `engine.Routes()` 双向比对 + 「永远拒绝」/「永远放行」两个 parser adapter 各跑一遍（详见下节）；审计只挂 `/api` |
| `internal/admin` | 后台鉴权、治理端点、与公开表面共用同一模块、评论固定视图 |

公共夹具：`pkg/testdb.Open`（每用例独立 `:memory:` 实例）、
`handler/test_helper_test.go` 的 `newTestApp`、`service/test_helper_test.go` 的依赖装配、
`repository/test_helper_test.go` 的建库/清库。

前端（`frontend/`）是另一套 Vitest 工具链，见下文「前端测试」一节，与 Go 测试互不包含。

## 运行测试

通用构建/门禁命令见 [docs/development.md](docs/development.md)（`go test ./...`、覆盖率、`make frontend-test` 等）。

测试专用的入口示例：

```bash
cd backend
go test -v ./model/...
go test -v ./pkg/jwt/...
go test -v ./service/...
go test -v ./handler/...
go test -v -run TestTokenBox ./pkg/jwt/...
go test -v -run TestAuth ./...
```

文档与分层门禁随 `go test ./...` 运行：`backend/internal/docscheck` 检查常驻文档的路径/链接/ADR 元数据/AGENTS 预算；`backend/internal/archtest` 检查 Go 包的依赖方向。

## 测试类型说明

本项目不在数据库上打桩：`repository` 与 `service` 用例通过 `testdb.Open(t)` 跑真实
SQLite `:memory:`；`handler` 用例用 `newTestApp(t)` 装配整条依赖链，再经 `httptest`
断言统一信封（HTTP 恒为 200，成败看 `code`）。这比手写 fake 更能挡住「fake 与 GORM
行为漂移，测试绿着但生产错」的问题（ADR-0007）。

前端是独立的 Vitest 工具链，测试约定见下文「前端测试」一节。

## 测试覆盖率

### 覆盖率目标

| 模块 | 目标覆盖率 | 说明 |
|------|-----------|------|
| model | > 90% | 数据模型和验证 |
| pkg | 每个**有测试文件的**包 > 85% | 公共组件；`logger`/`mail`/`testdb`/`version` 无测试文件，`./pkg/...` 合并口径必然低于门槛。`database` 的迁移与 pragma 测试见 `pkg/database/*_test.go` |
| service | > 80% | 业务逻辑 |
| handler | > 75% | API 接口 |

### 查看覆盖率报告

```bash
# 生成覆盖率报告
go test -coverprofile=coverage.out ./...

# 在浏览器中查看
go tool cover -html=coverage.out -o coverage.html
open coverage.html  # macOS
start coverage.html # Windows
```

## 测试辅助工具

### 数据库替身：`pkg/testdb`

Repository 与 service 测试**不写 mock**（见 ADR-0007）。每个测试调用 `testdb.Open(t)`
拿一个独立的 SQLite `:memory:` 实例，注入 repo/service；测试结束自动关闭。这样跑的是真 SQL
语义（含 `Preload`、分页、软删过滤），且测试互不污染业务状态。

> 「互不污染」不等于「可并行」：`service` 与 `handler` 两套夹具本身是包级变量
> （`testDB`、`testCodes`…），靠 go test 同包默认串行才安全。给这两包的用例加
> `t.Parallel()` 会让 `cleanupTestDB` 把另一个用例正在用的句柄置空。

```go
// 打开一个测试专用内存库并迁移表结构
db := testdb.Open(t) // *gorm.DB，仅本测试可见
```

### handler 装配：`newTestApp(t)`

handler 测试用一个 `newTestApp(t)` 把一整条依赖链（repo 实例 + service + `code.Store`
实例 + 记录型 `Mailer` + `jwt.TokenBox`）一次性装配好，返回 `*testApp`，其上挂着 `auth`、`user`、`account`、`comment`、
`log`、`uploads` 六个 handler 与 `tokens`、`db` 两个测试句柄（验证码存储不给句柄：用例一律经
`seedCode` 走真实下发链路，见下节）：

```go
func TestXxx(t *testing.T) {
    app := newTestApp(t)
    // app.auth / app.user / app.comment / app.log 是已装配的 handler
    // app.tokens 是本测试专用 token 单元；app.db 是底层 *gorm.DB
}
```

### 验证码：只经真实下发链路预置

`pkg/code.Store` 不提供「写入指定码」的入口。用例要一枚码，就走 `seedCode(t, flow, email)`
（service 包内同名助手）：它用**另一个** `VerificationCodes` 实例 `Send` 一次，从记录型
`Mailer` 里取回那枚码。要一枚必然错的码用 `seedCodeWrong`。

这样预置不需要知道存储键的命名空间格式（`"reg:"+email` 等）——那是验证码模块的私有细节，
漏到用例里就等于让每个用例都要跟着维护键格式。独立实例自带一对限流窗口，因此预置
既不占用被测服务的发码配额，写进的码又与被测服务共享同一个 `code.Store`。

`code.Store` 以实例注入（无全局单例），每个测试新建一个即可隔离；发码限流器随模块实例创建，
同样无包级共享状态。

### 路由表：`router/router_test.go`

路由表住在 `router` 包而不是 `main()`，因此它有 interface、能被测。三条独立的钉子：

1. **完整性**：`publicTable` / `adminTable` 各登记一路一路的 `method + pattern + 是否要登录`，
   用例把它和真实 `engine.Routes()` **双向比对**——多出来的路由（新加了却没登记）与登记了却
   找不到的路由都算失败。这一条才是「漏挂鉴权会在这里失败」的兑现处：只有强制每一路登记时
   回答一次「要不要登录」，忘了挂才无法藏在清单之外。
2. **反向（deny）**：注入**永远拒绝**的 `jwt.Parser` adapter，断言该挂鉴权的每一路都回 A0230、
   公开的那些都不回。
3. **正向（pass-through）**：再注入一个**永远放行**的 adapter，断言受保护的路**不**回 A0230。
   只有第 2 条挡不住「中间件漏挂、但 handler 自己 `CurrentUser` 也回 A0230」——两种故障长得
   一模一样，断言就成了同义反复；第 3 条要求 context 里真有身份，才能区分。

另有两条 ADR-0010 的隔离断言：公开引擎上不得出现 `/admin*`，后台引擎上也不得出现公开 `/api/*`。

`handler` 包内的测试仍各自挂自己需要的几路（那是用例夹具，不是一张平行路由表）。

### 请求取值：`pkg/httpx`

分页与数字 ID 的读取集中在 `pkg/httpx`，`params_test.go` 逐档钉住边界：缺省/非数字/越界
各归到哪一页，`0`、负数、超出 32 位的 ID 都算非法，`?user_id=`（未提供）与 `?user_id=abc`
（非法）必须分开——前者是「没有这个过滤条件」，后者要回 A0400。
断言里的边界值写成 `model.MaxPageSize`、`model.DefaultPageSize` 而不是字面量：改了 model
用例跟着走，若有人在 httpx 里偷偷另抄一份数值，用例就红。

端到端那一头由 `internal/admin` 的 `TestAdminListPagingUsesSiteDefault` 守住：后台省略
`page_size` 时拿到的条数与公开表面同源，防止后台再写一份自己的 `paginate`。

### 配置契约：Go 单一来源

`config/config.go` 的 `mapstructure` 标签定义字段，`config/schema.go` 定义默认值、环境变量
与校验规则；`config/schema_test.go` 用两条断言把它们钉住：

* `TestEveryConfigKeyIsRegisteredForViper` 保证 `Config` 的每个叶子键都登记进
  `fieldDefaults`（否则 `SetDefault`/`BindEnv` 都不会覆盖该键）；
* `TestValidationRulesRejectInvalidValues` 钉住「直接跑二进制也拒绝非法端口/通配监听」
  （ADR-0016 的 Go 单一来源与校验部分）。

### 审计：接入边界与有界捕获

* `router/router_test.go`：`/api` 路由组挂了审计，`/health`、文档与头像静态文件不经过；
* `service/log_test.go`：路径过滤规则（`/admin*`、`/health` 不是任何人的操作）；
* `middleware/operationlog_test.go`：写失败不阻塞业务响应、业务码被捕获、路径原样转发；
  `TestRecordWriter_BoundsJSONCapture` 断言 JSON 最多捕获 4 KiB 且截断不影响真实响应
  转发，`TestRecordWriter_SkipsNonJSON` 断言非 JSON 不占缓冲（ADR-0018）。

### 评论删除范围与后台视图

`service/comment_test.go` 的 `TestCommentService_Delete_*` 与
`TestCommentService_Delete_AdminScopeBypassesOwnership` 跑的是同一个 `Delete(id, scope)`：
仓储用条件 DELETE 并以 `RowsAffected` 判成功，没删到行时 service 再区分「不存在」与
「不是本人」。`TestCommentService_ListForAdmin_FixedView` 钉住后台固定视图（不含 email，
不序列化实体），`model/comment_test.go` 钉住两个 scope 的归属判定（ADR-0015）。

### 资料与凭据：文件与哈希的失败语义

* `service/profile_test.go`：用真实临时文件断言换头像删除旧文件、清头像同时清列与文件；
  同一文件仍被其他账号引用时保留（引用按 URL 的文件名判定，host 写法不同也算；头像 URL
  公开，不能靠客户端给的 URL 删别人文件——外部 host 伪造同一路径的用例专门钉住这条）；
  资料更新走列级 UPDATE，不整行 `Save`（否则登录失败计数会被一起覆盖）；
* `service/credentials_test.go`：`SetPassword` **先撤销 refresh token 再写新哈希**；
  撤销失败时保留旧哈希（不回退到「密码已改、会话未撤」）；失败计数只碰凭据列（ADR-0017）。

## 测试最佳实践

### 1. 测试命名规范

```go
func TestFunctionName_Scenario(t *testing.T) {
    // 测试逻辑
}

// 示例
func TestTokenBox_IssueUserShape(t *testing.T) {}
func TestTokenBox_ParseRejectsGarbage(t *testing.T) {}
func TestRegister_DuplicateUsername(t *testing.T) {}
```

### 2. 表驱动测试

```go
func TestValidateUsername(t *testing.T) {
    tests := []struct {
        name     string
        username string
        want     bool
    }{
        {"valid", "testuser", true},
        {"too short", "ab", false},
        {"too long", string(make([]byte, 51)), false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := ValidateUsername(tt.username); got != tt.want {
                t.Errorf("ValidateUsername() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### 3. 不要用 `t.Skip` 掩盖缺失夹具

Go 测试不依赖外部数据库，没有需要跳过的集成测试，也不要加 `t.Skip`。前端视图测试在
`fetch` 边界打桩，不依赖真实后端；需要真后端联调时按 `docs/development.md` 手工烟测。

### 4. 使用 httptest 测试 HTTP 接口

```go
func TestAPIEndpoint(t *testing.T) {
    app := newTestApp(t)
    router := setupRouter()
    router.POST("/api/auth/register", app.auth.Register)

    w := doJSON(t, router, http.MethodPost, "/api/auth/register", map[string]string{}, "")
    if w.Code != http.StatusOK {
        t.Errorf("HTTP = %d, want 200（业务成败看信封 code）", w.Code)
    }
    assertErrorCode(t, w.Body.Bytes(), "A0400")
}
```

## 前端测试（`frontend/`）

前端用 TypeScript + Vitest，与 Go 测试互不包含——**两边都要绿才算通过全量测试**。

```bash
make frontend-test   # 门禁：vue-tsc + vitest
# 或手动：
cd frontend
npm ci               # 已有 node_modules 时可跳过
npm run typecheck    # vue-tsc --noEmit
npm test             # vitest run
npm run build        # vite build（vite 不做类型检查，别拿它当校验）
```

### 两套测试环境

* 默认 `environment: 'node'`：`api`/`useAdminList` 等纯逻辑在 node 环境跑；
* 渲染组件的视图测试在文件首行标注 `// @vitest-environment happy-dom`
  （`src/views/AdminViews.test.ts`）。

### 取向：只在 fetch 边界打桩

沿用「能跑真东西就不打桩」（ADR-0007）的精神：

* `src/api.test.ts` 只 stub 全局 `fetch`：查询串拼装、信封判断、鉴权失败清 token 并跳转
  都是真代码；
* `src/views/AdminViews.test.ts` 用响应队列驱动三张列表：搜索、详情、分页、两步删除、
  日志筛选都走真实视图与 API 层；
* `src/useAdminList.test.ts` 覆盖成功/业务失败/空 data/网络异常/分页钳制。

### 覆盖点

后台 token 失效清退、snake_case 查询串与空值省略、列表分页与越界钳制、
用户搜索与详情、评论两步删除与失败保留行、日志方法与失败筛选、详情展开。

### 后端配置的 env 覆盖契约

`backend/config/config_test.go` 锁住一条容易被忽略的契约：`SMTP_PASSWORD` /
`JWT_SECRET` / `ADMIN_PASSWORD` 必须能覆盖 YAML。敏感项只放 `.env` 时，
`viper.Unmarshal` 只遍历它已知的键（默认值 / 配置文件 / `BindEnv`）——
既不在配置里、也没注册默认值的键，即使开了 `AutomaticEnv` 也读不到环境变量。

## 常见问题

### Q: 如何运行需要数据库的测试?

A: 直接 `go test ./...`。repository/service 用例各自通过 `testdb.Open(t)` 打开独立的
SQLite `:memory:` 实例，不需要外部数据库或手动准备 schema。前端测试用
`make frontend-test` 跑。

### Q: 如何添加新的测试?

A:

1. 在对应包创建 `*_test.go` 文件
2. 编写测试函数 (以 `Test` 开头)
3. 使用 `t.Error()` 或 `t.Fatal()` 报告错误
4. 运行 `go test ./...` 验证

### Q: 如何测试私有函数?

A: 测试文件可以访问同一包内的私有函数:

```go
// util.go
func privateFunc() string { return "private" }

// util_test.go
func TestPrivateFunc(t *testing.T) {
    result := privateFunc()
    if result != "private" {
        t.Errorf("Expected 'private', got '%s'", result)
    }
}
```

### Q: 如何模拟数据库?

A: **本项目不写数据库 mock。** 数据访问以 `service/ports.go` 里的 Store 接口隔开，
但测试用真实 SQLite `:memory:` 实例（`testdb.Open(t)`）当 adapter，而非手写 in-memory fake
——fake 一旦与 GORM 行为漂移，测试会「绿着但生产错」。唯一例外是 `Mailer`（无状态、两个真实
adapter），测试注入一个记录型 fake。理由与权衡见 [ADR-0007](docs/adr/0007-consumer-defined-repository-interfaces.md)。

## 测试检查清单

- [ ] 行为变化有回归用例，覆盖边界与错误处理
- [ ] 数据库相关用例使用 `testdb.Open` / `newTestApp`，没有手写 DB mock
- [ ] 缺失产物导致的 skip 已记录原因，没有当作通过
- [ ] 测试命名清晰描述场景
- [ ] 测试相互独立，不依赖执行顺序
- [ ] 测试数据在测试后清理

## 参考资源

- [Go Testing Package](https://pkg.go.dev/testing)
- [httptest Package](https://pkg.go.dev/net/http/httptest)
- [Go Test Coverage](https://blog.golang.org/coverage)
