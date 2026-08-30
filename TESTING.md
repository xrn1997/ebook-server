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
