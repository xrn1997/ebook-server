# ebook-server

一个基于 Go + Gin 的轻量级小说阅读器后端服务。

## 技术栈

- **语言**: Go 1.22
- **Web 框架**: Gin
- **ORM**: GORM
- **认证**: JWT
- **配置**: Viper
- **日志**: Zap
- **数据库**: SQLite（零配置，单文件数据库）

## 项目结构

```
ebook-server/
├── main.go              # 程序入口
├── config/              # 配置管理
├── middleware/           # 中间件
├── model/               # 数据模型
├── handler/             # API 处理器
├── service/             # 业务逻辑
├── repository/          # 数据访问
├── pkg/                 # 公共组件
│   ├── code/            # 验证码存储与校验
│   ├── database/        # 数据库连接初始化
│   ├── errcode/         # 五位业务码与统一响应
│   ├── jwt/             # JWT token 生成与解析
│   ├── logger/          # Zap 日志初始化
│   ├── mail/            # SMTP 邮件发送
│   └── ratelimit/       # 内存限流器
├── sql/                 # 数据库脚本（MySQL 参考，实际用 SQLite）
├── Dockerfile           # Docker 构建
└── Makefile             # 常用命令
```

## 快速开始

### 环境要求

- Go 1.22+
- 无需安装数据库（SQLite 自动创建）

### 安装步骤

1. **克隆项目**
   ```bash
   git clone <repository-url>
   cd ebook-server
   ```

2. **安装依赖**
   ```bash
   go mod tidy
   ```

3. **运行项目**
   ```bash
   go run main.go
   ```

   首次运行会自动创建 `ebook.db` 数据库文件。

### 使用 Makefile

```bash
# 构建
make build

# 运行
make run

# 清理
make clean

# Docker 构建
make docker
```

## API 接口

### 认证相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | /api/auth/send-code | 按邮箱发送验证码 | 否 |
| POST | /api/auth/register | 注册（邮箱+验证码+密码），激活建号，不发 token | 否 |
| POST | /api/auth/login | 邮箱+密码登录，返回双 token | 否 |
| POST | /api/auth/refresh | 刷新 token（轮换） | 否（用 refresh token） |
| POST | /api/auth/logout | 登出 | 是 |
| POST | /api/auth/forgot-password/send-code | 忘记密码发邮箱验证码 | 否 |
| POST | /api/auth/forgot-password/reset | 验证码重置密码 | 否 |

> **账号模型**：`email` 是登录主标识（唯一必填），`uid` 是主键，`username` 非空但
> 可重复（注册时自动生成，可后改）。详见 [ADR-0002](docs/adr/0002-email-based-registration-and-account-model.md)。

### 用户相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/users/me | 获取当前用户信息 | 是 |
| PUT | /api/users/me | 更新用户信息 | 是 |
| PUT | /api/users/me/password | 已登录修改密码 | 是 |

### 评论相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/comments | 获取评论列表 | 否 |
| POST | /api/comments | 创建评论 | 是 |
| GET | /api/comments/my | 获取我的评论 | 是 |
| DELETE | /api/comments/:id | 删除评论 | 是 |

### 日志相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/logs | 获取操作日志 | 是 |
| GET | /api/logs/my | 获取我的操作日志 | 是 |

## 请求示例

> **统一响应信封**：所有业务端点 HTTP 状态码恒为 200，通过 `code` 业务码区分成败
> （`"00000"` 表示成功），错误文案位于 `error` 字段，可直达移动端 Toast。

### 发送验证码

前端点击「发送验证码」时调用：

```bash
curl -X POST http://localhost:8080/api/auth/send-code \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com"}'
```

### 注册

```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "code": "123456",
    "password": "123456"
  }'
```

注册成功（激活建号，**不发 token**），返回 `code 00000`。

### 登录

```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "123456"
  }'
```

登录成功（返回双 token）：

```json
{
  "code": "00000",
  "error": "",
  "data": {
    "token": "eyJ...",
    "refresh_token": "a1b2c3...",
    "user": {
      "uid": 1,
      "email": "test@example.com",
      "username": "user_1a2b3c4d",
      "nickname": "user_1a2b3c4d",
      "avatar": ""
    }
  }
}
```

### 刷新 token

```bash
curl -X POST http://localhost:8080/api/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "a1b2c3..."}'
```

### 已登录修改密码

```bash
curl -X PUT http://localhost:8080/api/users/me/password \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access-token>" \
  -d '{"old_password": "123456", "new_password": "654321"}'
```

### 获取用户信息

```bash
curl http://localhost:8080/api/users/me \
  -H "Authorization: Bearer <your-token>"
```

### 创建评论

```bash
curl -X POST http://localhost:8080/api/comments \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "content": "这是一条评论"
  }'
```

## Docker 部署

### 构建镜像

```bash
docker build -t ebook-server .
```

### 运行容器

```bash
docker run -p 9090:9090 ebook-server
```

## 配置说明

配置文件 `config.yaml`：

```yaml
server:
  port: 9090            # 服务端口（config.yaml 默认 9090；代码默认 8080）
  mode: debug           # 运行模式: debug, release, test

database:
  path: ebook.db      # SQLite 数据库文件路径

jwt:
  secret: your-secret-key  # JWT 密钥（请修改）
  expire_min: 120          # access token 过期时间（分钟）

smtp:
  host: smtp.example.com   # SMTP 服务器（留空则验证码写日志）
  port: 465                # 465 走 TLS；587 走 STARTTLS
  username: no-reply@example.com
  password: change-me      # 真实授权码放 .env 的 SMTP_PASSWORD
  from: no-reply@example.com
  insecure: false
```

> 敏感项走本地 `.env`（已被 git 排除）：`SMTP_PASSWORD` 覆盖 `smtp.password`，
> `JWT_SECRET` 覆盖 `jwt.secret`。见 `.env.example`。

## 开发说明

### 添加新接口

1. 在 `model/` 中定义数据模型
2. 在 `repository/` 中实现数据访问
3. 在 `service/` 中实现业务逻辑
4. 在 `handler/` 中实现 API 处理
5. 在 `main.go` 中注册路由

### 数据库迁移

项目使用 GORM 自动迁移，启动时会自动创建/更新表结构。

## 测试

### 运行测试

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

# 运行指定包的测试
go test -v ./model/...
go test -v ./service/...
go test -v ./handler/...

# 运行指定测试函数
go test -v -run TestGenerateToken ./pkg/jwt/...
```

### 测试结构

```
ebook-server/
├── model/
│   └── model_test.go          # 模型测试
├── pkg/
│   ├── code/
│   │   └── store_test.go      # 验证码存储测试
│   ├── errcode/
│   │   └── errcode_test.go    # 业务码测试
│   ├── jwt/
│   │   └── jwt_test.go        # JWT 工具测试
│   └── ratelimit/
│       └── limiter_test.go    # 限流器测试
├── middleware/
│   ├── auth_test.go           # 认证中间件测试
│   ├── cors_test.go           # 跨域中间件测试
│   └── recovery_test.go       # 恢复中间件测试
├── repository/
│   ├── user_test.go           # 用户仓库测试
│   ├── comment_test.go        # 评论仓库测试
│   ├── log_test.go            # 日志仓库测试
│   └── refresh_token_test.go  # 刷新令牌仓库测试
├── service/
│   ├── auth_test.go           # 认证服务测试
│   ├── user_test.go           # 用户服务测试
│   ├── comment_test.go        # 评论服务测试
│   └── log_test.go            # 日志服务测试
└── handler/
    ├── auth_test.go           # 认证接口测试
    ├── user_test.go           # 用户接口测试
    ├── comment_test.go        # 评论接口测试
    └── log_test.go            # 日志接口测试
```

### 测试覆盖率目标

- **model**: > 90%
- **pkg**: > 85%
- **service**: > 80%
- **handler**: > 75%

详细测试文档请查看 [TESTING.md](TESTING.md)

### 测试类型

1. **单元测试**: 测试单个函数/方法的逻辑
2. **集成测试**: 测试模块间的交互
3. **接口测试**: 测试 HTTP API 的请求和响应

### 使用 testify 增强测试

```bash
go get -u github.com/stretchr/testify
```

```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSomething(t *testing.T) {
    assert.Equal(t, expected, actual)
    require.NoError(t, err)
}
```

## License

MIT
