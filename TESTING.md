# 测试文档

本文档描述了 ebook-server 项目的单元测试策略和实现。

## 测试概述

### 测试目标

- **代码覆盖率**: 整体 > 80%
- **关键模块覆盖率**: model > 90%, pkg > 85%, service > 80%, handler > 75%
- **测试类型**: 单元测试、集成测试

### 测试框架

- Go 标准测试包 `testing`
- HTTP 测试 `net/http/httptest`
- 可选: testify (用于断言增强)

## 测试结构

```
ebook-server/
├── model/
│   └── model_test.go              # 模型测试
├── pkg/
│   ├── code/
│   │   └── store_test.go          # 验证码存储测试
│   ├── errcode/
│   │   └── errcode_test.go        # 业务码测试
│   ├── jwt/
│   │   └── jwt_test.go            # JWT 工具测试
│   └── ratelimit/
│       └── limiter_test.go        # 限流器测试
├── middleware/
│   ├── auth_test.go               # 认证中间件测试
│   ├── cors_test.go               # 跨域中间件测试
│   └── recovery_test.go           # 恢复中间件测试
├── repository/
│   ├── user_test.go               # 用户仓库测试
│   ├── comment_test.go            # 评论仓库测试
│   ├── log_test.go                # 日志仓库测试
│   ├── refresh_token_test.go      # 刷新令牌仓库测试
│   └── test_helper_test.go       # 测试数据库初始化工具（*_test.go，仅测试可见）
├── service/
│   ├── auth_test.go               # 认证服务测试
│   ├── user_test.go               # 用户服务测试
│   ├── comment_test.go            # 评论服务测试
│   ├── log_test.go                # 日志服务测试
│   ├── account_test.go            # 账号导出/注销服务测试
│   ├── enumeration_test.go        # 防账号枚举服务测试
│   └── test_helper_test.go        # 测试依赖装配（service 层注入 :memory: 实例）
├── handler/
│   ├── auth_test.go               # 认证接口测试
│   ├── user_test.go               # 用户接口测试
│   ├── comment_test.go            # 评论接口测试
│   ├── log_test.go                # 日志接口测试
│   ├── deletion_test.go           # 账号注销/导出接口测试
│   ├── enumeration_test.go        # 防账号枚举接口测试
│   └── test_helper_test.go        # 测试辅助工具（newTestApp 装配整条 handler 链）
└── TESTING.md                     # 本文档
```

## 运行测试

### 基本命令

```bash
# 运行所有测试
go test ./...

# 运行测试并显示详细信息
go test -v ./...

# 运行测试并生成覆盖率报告
go test -cover ./...

# 生成详细的覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### 运行指定包的测试

```bash
# 模型测试
go test -v ./model/...

# JWT 工具测试
go test -v ./pkg/jwt/...

# 服务层测试
go test -v ./service/...

# 接口层测试
go test -v ./handler/...
```

### 运行指定测试函数

```bash
# 运行 JWT 相关测试
go test -v -run TestGenerateToken ./pkg/jwt/...

# 运行认证相关测试
go test -v -run TestAuth ./...

# 运行用户相关测试
go test -v -run TestUser ./...

# 运行评论相关测试
go test -v -run TestComment ./...
```

### 使用 Makefile

```bash
# 运行所有测试
make test

# 运行测试并显示详细信息
make test-verbose

# 运行测试并生成覆盖率报告
make test-coverage

# 运行指定包的测试
make test-model
make test-pkg
make test-service
make test-handler
```

## 测试类型说明

### 1. 单元测试

测试单个函数或方法的逻辑，不依赖外部服务。

**示例**: `model/model_test.go`

```go
func TestUserTableName(t *testing.T) {
    user := User{}
    if user.TableName() != "users" {
        t.Errorf("Expected 'users', got '%s'", user.TableName())
    }
}
```

### 2. 集成测试

测试模块间的交互，通常需要数据库连接。

**示例**: `service/auth_test.go`

```go
func TestAuthService_Register_Success(t *testing.T) {
    t.Skip("Skipping integration test - requires database")
    // 测试代码...
}
```

### 3. 接口测试

测试 HTTP API 的请求和响应。

**示例**: `handler/auth_test.go`

```go
func TestAuthHandler_Register_InvalidBody(t *testing.T) {
    router := setupRouter()
    authHandler := NewAuthHandler()
    router.POST("/api/auth/register", authHandler.Register)

    req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte("{}")))
    req.Header.Set("Content-Type", "application/json")

    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Errorf("Expected status 400, got %d", w.Code)
    }
}
```

## 测试覆盖率

### 覆盖率目标

| 模块 | 目标覆盖率 | 说明 |
|------|-----------|------|
| model | > 90% | 数据模型和验证 |
| pkg | > 85% | 公共组件 |
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
语义（含 `Preload`、分页、软删过滤），且测试互不污染、可并行。

```go
// 打开一个测试专用内存库并迁移表结构
db := testdb.Open(t) // *gorm.DB，仅本测试可见
```

### handler 装配：`newTestApp(t)`

handler 测试用一个 `newTestApp(t)` 把一整条依赖链（repo 实例 + service + `code.Store`
实例 + 写日志的 `Mailer`）一次性装配好，返回 `*testApp`，其上挂着 `auth`、`user`、`account`、`comment`、
`log` 五个 handler 与 `codes`、`db` 两个测试句柄：

```go
func TestXxx(t *testing.T) {
    app := newTestApp(t)
    // app.auth / app.user / app.comment / app.log 是已装配的 handler
    // app.codes 是注入的验证码存储，可直接塞码；app.db 是底层 *gorm.DB
}
```

### 验证码与限流

`code.Store` 以实例注入（无全局单例），每个测试 `new` 一个独立存储即可隔离；
发码限流器随 service 实例创建，同样无包级共享状态。

## 测试最佳实践

### 1. 测试命名规范

```go
func TestFunctionName_Scenario(t *testing.T) {
    // 测试逻辑
}

// 示例
func TestGenerateToken_Success(t *testing.T) {}
func TestParseToken_InvalidToken(t *testing.T) {}
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

### 3. 跳过集成测试

```go
func TestDatabaseOperation(t *testing.T) {
    t.Skip("Skipping integration test - requires database")
    // 测试逻辑...
}
```

### 4. 使用 httptest 测试 HTTP 接口

```go
func TestAPIEndpoint(t *testing.T) {
    router := setupRouter()
    // 注册路由...

    req, _ := http.NewRequest("GET", "/api/endpoint", nil)
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("Expected status 200, got %d", w.Code)
    }
}
```

## 桌面应用测试（`desktop/`）

桌面应用有独立 npm 工具链（Vitest），与 Go 测试互不包含——**两边都要绿才算通过全量测试**。

```bash
make desktop-test        # 门禁：先编译 sidecar，再 typecheck + vitest（推荐）
# 或手动：
cd desktop
npm install
npm test                # vitest run
npm run typecheck       # 主进程 + renderer 各一套 tsconfig
npm run build           # tsc + vite build（vite 不做类型检查，别拿它当校验）
```

> **为什么门禁要先用 make**：`backend-integration.test.ts`（真起 Go 子进程的集成用例）
> 在 sidecar 二进制缺失时整段 `describe.skip`——fresh clone 直接 `npm test` 会得到
> 「绿但没测到真后端」的假阴性。`make desktop-test` 先执行
> `desktop-build-backend` 保证二进制存在，集成层不会静默缺席。

### 两套测试环境

* 默认 `environment: 'node'`：主进程模块（`sidecar`/`config`/`ipc`/`admin-auth`）与
  纯逻辑模块（`api`/`useAdminList`/`format`）都在 node 环境跑；
* 需要渲染组件的视图测试在文件首行标注 `// @vitest-environment happy-dom`
  （如 `src/renderer/tests/Overview.test.ts`、`Config.test.ts`、`AdminViews.test.ts`）。

### 取向：能跑真东西就不打桩

沿用后端「不写 mock、跑真语义」（ADR-0007）的精神：

* `src/main/__tests__/backend-integration.test.ts` 起**真实 Go 子进程**：健康检查、
  后台自动登录、用户/评论/日志/统计接口拉通、改配置重启后健康检查拨新端口、
  停止耗时远小于 5 秒（真优雅退出）——这是设计文档「关键用户流程」的集成层兑现；
* `admin-auth.test.ts` 起**真实 HTTP 假后台**（端口 0 由系统分配），
  验证统一信封能被解开、令牌合流与重取确实只发一次登录——这条链路当初就是因为
  没人真跑过请求而整条不通；
* `config.test.ts` 用 `fs.mkdtemp` 走**真实文件系统**，断言敏感字段只落 `.env`、
  往返后完整还原、用户自加的 env 键不被抹掉，并验证通配监听地址（含 `[::]`）被拒存；
* `AdminViews.test.ts`（用户/评论/日志三视图）打桩的只有 `window.electronAPI` 与
  全局 fetch，查询串拼装、信封判断、两步删除确认、分页钳制都是真代码；
* `ipc.test.ts` 打桩的只有 `electron.ipcMain`（收集注册的 handler 后直接调用），
  配置读写仍是真实文件；
* 只有 `sidecar.test.ts` 需要 mock `node:child_process` 与 `node:http`：子进程 stdin
  优雅退出指令与 10 秒超时必须被控制。它显式传 `platform`，否则平台相关断言
  会随宿主平台漂移。

### E2E 冒烟测试（Playwright `_electron`）

```bash
make desktop-e2e           # 先编译 sidecar + renderer 产物，再跑 Playwright
# 或手动：
cd desktop
npm run build:main && npm run test:preload && npx playwright test
```

E2E 用 Playwright 的 `_electron.launch` 直接驱动本机 `node_modules/electron` 运行时，
不需要下载浏览器二进制。测试把 `userData` 指到临时目录（`EBOOK_SERVER_USER_DATA`），
renderer 走打包后的 `dist/renderer/index.html`（`EBOOK_SERVER_RENDERER_FILE`），
避免污染真实用户数据或依赖 vite dev server。

**前置条件**：sidecar 二进制（`desktop/resources/backend/ebook-server.exe`）与
renderer 产物（`desktop/dist/renderer/index.html`）都要存在。产物缺失时整段 `test.skip`，
与「裸 `npm test` 会让集成用例因二进制缺失整段跳过」的门禁逻辑一致——
`make desktop-e2e` 先跑 `desktop-build-backend` 与 `desktop-build-frontend` 保证产物齐备。

E2E 项目有独立的 `tsconfig.e2e.json`（不并入主 typecheck 的 `rootDir: "src"`，
否则 `e2e/` 会触发 TS6059），`typecheck` 脚本末尾追加 `tsc -p tsconfig.e2e.json --noEmit`；
vitest 的 `exclude` 已把 `e2e/**` 排除，两套测试互不干扰。

### 覆盖点

sidecar 生命周期与崩溃重启退避、端口每次启动重新求值、stdin 优雅退出与 5 秒强杀兜底、
stopping 期间不重复启动、停止不得复活进程、运行时长、配置结构收敛与未知键丢弃、
ADR-0010 通配地址拒存（含 `[::]`）、IPC 通道契约、后台自动登录与令牌失效重取、
renderer 信封判断与失败文案透出、列表分页与越界钳制、评论两步删除、
用户详情、日志筛选、概览页运行监控渲染。

### 后端配置的 env 覆盖契约

`backend/config/config_test.go` 锁住一条容易被忽略的契约：`SMTP_PASSWORD` /
`JWT_SECRET` / `ADMIN_PASSWORD` 必须能覆盖 YAML。桌面应用把密钥从 `config.yaml` 拆到
`.env`，而 `viper.Unmarshal` 只遍历它已知的键——没注册默认值的键即使开了
`AutomaticEnv` 也读不到环境变量（详见 ADR-0012 §2）。

## 常见问题

### Q: 如何运行需要数据库的测试?

A: 集成测试默认被跳过。要运行它们:

1. 确保测试数据库存在
2. 移除 `t.Skip()` 调用
3. 运行 `go test -v ./service/...`

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

- [ ] 每个函数都有对应的测试
- [ ] 测试覆盖正常流程和边界情况
- [ ] 测试覆盖错误处理
- [ ] 集成测试被正确跳过
- [ ] 测试命名清晰描述测试场景
- [ ] 测试相互独立，不依赖执行顺序
- [ ] 测试数据在测试后清理

## 参考资源

- [Go Testing Package](https://pkg.go.dev/testing)
- [httptest Package](https://pkg.go.dev/net/http/httptest)
- [Testify Assertions](https://pkg.go.dev/github.com/stretchr/testify/assert)
- [Go Test Coverage](https://blog.golang.org/coverage)
