# ebook-server

一个基于 Go + Gin 的轻量级小说阅读器后端服务，配套 Android 客户端
[android-ebook](https://github.com/xrn1997/android-ebook)。仓库为
**前后端 monorepo**（[ADR-0009](docs/adr/0009-monorepo-backend-and-admin-frontend.md)）：
Go 后端 + Vue 后台管理前端，构建后前端内嵌进单一可执行文件。

## 技术栈

- **语言**: Go 1.22
- **Web 框架**: Gin
- **ORM**: GORM + SQLite（glebarez/sqlite，纯 Go 驱动，零配置）
- **认证**: JWT（用户 token 与后台管理 token 相互隔离）
- **配置**: Viper（YAML + `.env` 覆盖）
- **日志**: Zap
- **数据库**: SQLite（单文件数据库，AutoMigrate 自动建表）
- **后台前端**: Vue 3 + Vite（构建产物 go:embed 进单 exe）

## 项目结构

```
ebook-server/
├── backend/               # Go 后端（module 根，全部 Go 代码在此）
│   ├── main.go            # 程序入口（依赖装配唯一装配点，ADR-0007）
│   ├── config/            # 配置管理
│   ├── internal/admin/    # 后台管理系统（独立表面、独立鉴权、内嵌前端）
│   ├── middleware/        # 中间件（JWT 认证、CORS、日志、操作审计、panic 恢复）
│   ├── model/             # 数据模型（GORM 实体、请求/响应结构体、错误）
│   ├── handler/           # HTTP 处理器
│   ├── service/           # 业务逻辑（事务编排、业务规则）
│   ├── repository/        # 数据访问（GORM 查询）
│   ├── pkg/               # 公共组件
│   │   ├── code/          # 验证码存储与校验
│   │   ├── database/      # 数据库连接初始化
│   │   ├── errcode/       # 五位业务码与统一响应
│   │   ├── jwt/           # JWT token 生成与解析
│   │   ├── logger/        # Zap 日志初始化
│   │   ├── mail/          # SMTP 邮件发送
│   │   ├── ratelimit/     # 内存限流器
│   │   └── upload/        # 头像文件存储与公开访问（ADR-0011）
│   └── docs/              # swag 生成的 Swagger 文档（make docs 更新）
├── frontend/              # 后台管理前端（Vue 3 + Vite，构建后灌入 backend/internal/admin/web）
├── docs/                  # ADR 设计决策记录
├── sql/                   # 数据库脚本（MySQL 参考，实际用 SQLite）
├── config.yaml            # 运行配置
├── Dockerfile             # Docker 构建
└── Makefile               # 常用命令
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
   cd backend && go mod tidy
   ```

3. **运行项目**
   ```bash
   make run    # 等价于 cd backend && go run main.go
   ```

   首次运行会自动创建 `ebook.db` 数据库文件与 `uploads/` 目录。

### 使用 Makefile

```bash
make build    # 前端 build → 灌 dist → go build，产出单 exe（内嵌后台前端）
make run      # 运行
make docs     # 重新生成 Swagger 文档（swag init，需先安装 swag）
make clean    # 清理
make docker   # Docker 构建
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
| PUT | /api/users/me | 更新用户信息（`email` 不可变更，传不同值返回 `A0113`） | 是 |
| PUT | /api/users/me/password | 已登录修改密码 | 是 |
| GET | /api/users/me/data | 导出我的数据（用户资料 + 本人评论） | 是 |
| POST | /api/users/me/deletion/send-code | 发送注销验证码到当前账号邮箱 | 是 |
| POST | /api/users/me/deletion | 注销账号（验证码确认，匿名化并返回数据副本） | 是 |

> **账号注销说明**（[ADR-0005](docs/adr/0005-account-deletion-by-anonymization.md)）
>
> * 注销采用**匿名化**而非删除：改写 `email` 为占位值、清空密码与头像，账号不可再登录
> * 同一邮箱注销后可以**重新注册**（占位 email 释放了唯一索引）
> * **公开评论与操作日志保留**——评论作者将显示为"已注销用户"
> * **不可撤销**：注销前请先用 `GET /api/users/me/data` 导出数据，注销响应也会带上同一份副本

### 评论相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/comments | 评论列表，支持 `chapter_url`/`book_name` 章节过滤 | 否 |
| POST | /api/comments | 创建评论，支持章节归属（`chapter_url`/`chapter_name`/`book_name`，可选） | 是 |
| GET | /api/comments/my | 获取我的评论 | 是 |
| DELETE | /api/comments/:id | 删除评论（仅本人，非本人返回 `A0303`） | 是 |

> **评论章节归属**（[ADR-0011](docs/adr/0011-comment-chapter-and-avatar-upload.md)）：
> 评论按书源章节 URL 组织（`chapter_url` 为聚合键，`chapter_name`/`book_name` 为展示快照，
> 后端原样存储、不校验格式）。不传章节字段 = 书籍级评论，兼容既有数据。
> 评论响应用独立视图：`user` 只含 `uid/username/nickname/avatar`（不含 email），
> `add_time` 固定上海时区 `yyyy-MM-dd HH:mm:ss` 格式。
> 评论域错误码：`A0303` 无权删除、`A0304` 评论不存在（`A0301`/`A0302` 预留）。

### 头像上传

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | /api/uploads/avatar | 上传头像（multipart，字段名 `avatar`，jpg/png/webp ≤5MB），返回图片 URL | 是 |
| GET | /uploads/* | 上传文件的静态访问（仅头像等公开资源） | 否 |

> **头像两步提交**（ADR-0011）：先 `POST /api/uploads/avatar` 拿到 `{url}`，
> 再 `PUT /api/users/me` 携带 `{"avatar": "<url>"}` 完成更新；更换头像时旧文件自动清理。

### 日志相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/logs | 获取操作日志 | 是 |
| GET | /api/logs/my | 获取我的操作日志 | 是 |

## 后台管理系统

后台管理（Vue 前端 + API）挂 `/admin`，与公开 API **完全分离**（[ADR-0009](docs/adr/0009-monorepo-backend-and-admin-frontend.md)）：

- **独立鉴权**：管理端 token 用独立的 `admin.jwt_secret` 签发，与用户 token 互不可用
- **网络隔离**（[ADR-0010](docs/adr/0010-admin-network-isolation.md)）：后台独立监听
  `admin.listen_addr:admin.listen_port`（默认 `127.0.0.1:9091`），**公网物理上无法连接**；
  远程管理请走 SSH 隧道 / VPN，不要直接开放后台端口
- **登录限流**：后台登录 5 次/分钟/来源 IP，超限返回 `A0241`
- 本机访问：`http://127.0.0.1:9091/admin`（账号密码见 `config.yaml` 的 `admin` 段，生产必须修改）
- 后台引擎自带 Swagger 文档（`http://127.0.0.1:9091/api-docs/`），供本机查阅

## 请求示例

> **统一响应信封**：所有业务端点 HTTP 状态码恒为 200，通过 `code` 业务码区分成败
> （`"00000"` 表示成功），错误文案位于 `error` 字段，可直达移动端 Toast。

### 发送验证码

前端点击「发送验证码」时调用：

```bash
curl -X POST http://localhost:9090/api/auth/send-code \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com"}'
```

### 注册

```bash
curl -X POST http://localhost:9090/api/auth/register \
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
curl -X POST http://localhost:9090/api/auth/login \
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
curl -X POST http://localhost:9090/api/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "a1b2c3..."}'
```

### 已登录修改密码

```bash
curl -X PUT http://localhost:9090/api/users/me/password \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access-token>" \
  -d '{"old_password": "123456", "new_password": "654321"}'
```

### 获取用户信息

```bash
curl http://localhost:9090/api/users/me \
  -H "Authorization: Bearer <your-token>"
```

### 创建评论

```bash
curl -X POST http://localhost:9090/api/comments \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "content": "这是一条评论"
  }'
```

带章节归属（可选字段，不传则为书籍级评论）：

```bash
curl -X POST http://localhost:9090/api/comments \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "content": "这一章的剧情真精彩",
    "chapter_url": "https://src.example.com/book/42/5.html",
    "chapter_name": "第五章",
    "book_name": "示例之书"
  }'
```

### 查询章节评论

```bash
curl "http://localhost:9090/api/comments?chapter_url=https%3A%2F%2Fsrc.example.com%2Fbook%2F42%2F5.html&page=1&page_size=20"
```

> 列表统一返回分页包裹结构 `{items, total, page, page_size}`，按 `add_time` 倒序。

### 上传头像（两步提交）

第一步上传拿 URL：

```bash
curl -X POST http://localhost:9090/api/uploads/avatar \
  -H "Authorization: Bearer <your-token>" \
  -F "avatar=@/path/to/avatar.png"
```

返回 `{"url": "http://<host>:9090/uploads/avatar/xxxx.png"}`，第二步提交到用户资料：

```bash
curl -X PUT http://localhost:9090/api/users/me \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{"avatar": "http://<host>:9090/uploads/avatar/xxxx.png"}'
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

> 后台管理（`/admin`）默认监听容器内 `127.0.0.1:9091`，**不从容器外部暴露**（安全默认）。
> 如需外部访问后台，改 `admin.listen_addr` 为 `0.0.0.0` 并映射端口——仅限内网受信环境。

## 配置说明

配置文件 `config.yaml`：

```yaml
server:
  port: 9090            # 服务端口（config.yaml 与代码默认值一致，均为 9090）
  mode: debug           # 运行模式: debug, release, test

database:
  path: ebook.db      # SQLite 数据库文件路径

api_docs:
  enabled: false      # 公开 API 端口是否提供 Swagger 文档（默认关，防接口清单泄露；联调时改 true）

jwt:
  secret: your-secret-key  # JWT 密钥（请修改）
  expire_min: 120          # access token 过期时间（分钟）

admin:
  username: admin            # 后台管理系统账号
  password: change-me        # 后台密码（真实值放 .env 的 ADMIN_PASSWORD）
  jwt_secret: your-admin-secret  # 管理端 JWT 密钥（.env 的 ADMIN_JWT_SECRET 覆盖）
  expire_min: 60             # 管理端 token 有效期（分钟）
  listen_addr: 127.0.0.1     # 后台监听地址：默认仅本机（公网不可达）；局域网访问改内网 IP
  listen_port: 9091          # 后台监听端口（与公开 API 的 9090 分离）

smtp:
  host: smtp.example.com   # SMTP 服务器（留空则验证码写日志）
  port: 465                # 465 走 TLS；587 走 STARTTLS
  username: no-reply@example.com
  password: change-me      # 真实授权码放 .env 的 SMTP_PASSWORD
  from: no-reply@example.com
  insecure: false

upload:
  dir: uploads             # 上传文件根目录（头像存 <dir>/avatar/，经 /uploads/* 公开）
```

> 敏感项走本地 `.env`（已被 git 排除）：`SMTP_PASSWORD` 覆盖 `smtp.password`，
> `JWT_SECRET` 覆盖 `jwt.secret`。见 `.env.example`。

## 开发说明

### 添加新接口

Go 代码全部位于 `backend/` 下：

1. 在 `backend/model/` 中定义数据模型
2. 在 `backend/repository/` 中实现数据访问
3. 在 `backend/service/` 中实现业务逻辑
4. 在 `backend/handler/` 中实现 API 处理（含 Swagger 注解）
5. 在 `backend/main.go` 中注册路由
6. 编写对应测试（见测试结构）
7. 修改接口注解后运行 `make docs` 重新生成 Swagger 文档

> 详细规范见 [AGENTS.md](AGENTS.md)（分层约定、提交规范、已知问题等）。

### 数据库迁移

项目使用 GORM 自动迁移，启动时会自动创建/更新表结构。

## 测试

### 运行测试

```bash
# 运行所有测试（Go 代码在 backend/ 下）
cd backend
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
go test -v ./pkg/upload/...

# 运行指定测试函数
go test -v -run TestGenerateToken ./pkg/jwt/...
```

### 测试结构

```
backend/
├── model/
│   └── model_test.go          # 模型测试
├── pkg/
│   ├── code/
│   │   └── store_test.go      # 验证码存储测试
│   ├── errcode/
│   │   └── errcode_test.go    # 业务码测试
│   ├── jwt/
│   │   └── jwt_test.go        # JWT 工具测试
│   ├── ratelimit/
│   │   └── limiter_test.go    # 限流器测试
│   └── upload/
│       └── upload_test.go     # 头像存储测试（校验/命名/删除/防穿越）
├── middleware/
│   ├── auth_test.go           # 认证中间件测试
│   ├── cors_test.go           # 跨域中间件测试
│   └── recovery_test.go       # 恢复中间件测试
├── repository/
│   ├── user_test.go           # 用户仓库测试
│   ├── comment_test.go        # 评论仓库测试（含章节过滤）
│   ├── log_test.go            # 日志仓库测试
│   └── refresh_token_test.go  # 刷新令牌仓库测试
├── service/
│   ├── auth_test.go           # 认证服务测试
│   ├── user_test.go           # 用户服务测试
│   ├── comment_test.go        # 评论服务测试（含章节归属/过滤）
│   └── log_test.go            # 日志服务测试
└── handler/
    ├── auth_test.go           # 认证接口测试
    ├── user_test.go           # 用户接口测试
    ├── comment_test.go        # 评论接口测试（含章节/删除权限）
    ├── upload_test.go         # 头像上传接口测试
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
