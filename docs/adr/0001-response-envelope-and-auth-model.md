# 响应信封重构 + 认证模型升级

> **注意**：本文「账号体系」「密码管理」两节已被
> [ADR-0002](0002-email-based-registration-and-account-model.md) 更迭
> （注册改为「邮箱 + 验证码 + 密码」、`email` 为登录主标识）。响应信封与
> 双 token 部分仍以本文为准。

将响应信封从 `{code: Int, message, data}` 改为 `{code: String, error, data}`，
HTTP 状态码恒为 200；同时引入双 token 机制、用户名+邮箱账号体系、密码找回流程。

## 现状

### 响应信封

`model/response.go` 定义：

```go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

业务错误（如密码错误）通过 HTTP 状态码表达：`BadRequest` 返回 400，
`Unauthorized` 返回 401。`handler/auth.go` 的 `Login` 失败时调用
`model.Unauthorized(c, err.Error())`，HTTP 状态码为 401。

### 认证

- 单 token（JWT，72 小时过期），登录返回 `{token, username, email}`
- 无 refresh token，token 过期后用户必须重新登录
- 密码用 bcrypt 哈希存储
- 注册邮箱可选（`binding:"omitempty,email"`）

### 账号标识

`RegisterRequest` 和 `LoginRequest` 均以 `username` 为主标识。
数据库 `users` 表已有 `email` 字段但非必填。无手机号字段。

## 动机

### 信封重构

移动端 HTTP 客户端（Retrofit）对非 2xx 响应直接抛异常，响应体无法到达业务
解析层。具体问题：

1. 后端返回 401 + `{code: 401, message: "密码错误"}` → Retrofit 抛
   `HttpException` → 响应体丢失 → 用户看到的是兜底文案"操作未授权"而非
   "密码错误"
2. 业务错误文案无法直达用户 Toast，丢失了服务端精心构造的错误信息

改为 HTTP 200 + 业务码后，所有响应都能到达业务解析层，错误文案可直达用户。

### 双 token

单 token 有效期 72 小时，过期即强制重新登录。对于日常使用的阅读 App，
用户期望"打开就能用"。双 token（access 短效 + refresh 长效）可在
refresh 有效期内无感续期。

### 邮箱必填

密码找回需要邮件通道。当前邮箱可选导致部分用户无法找回密码。

## 决策

### 1. 响应信封

新信封结构：

```go
type Response struct {
    Code  string      `json:"code"`  // 五位业务码，"00000" 表示成功
    Error string      `json:"error"` // 业务错误文案（成功时为空串）
    Data  interface{} `json:"data,omitempty"`
}
```

HTTP 状态码规则：
- **200**：业务层有响应（无论成败），所有业务端点一律返回 200
- **非 200**：仅用于网关故障、进程崩溃等传输层异常（本项目为单进程，
  正常运行不会出现非 200）

### 2. 业务错误码

建立 `pkg/errcode` 包，定义五位字符串业务码。码位规划：

| 码      | 含义               | 使用场景           |
|---------|-------------------|-------------------|
| `00000` | 成功               | 所有成功响应        |
| `A0110` | 用户名校验失败       | 用户名格式不合规     |
| `A0111` | 用户名已存在         | 注册时用户名冲突     |
| `A0120` | 密码校验失败         | 密码格式不合规       |
| `A0132` | 验证码错误           | 找回密码验证码不匹配  |
| `A0153` | 邮箱格式错误         | 邮箱格式不合规       |
| `A0201` | 账户不存在           | 登录/找回密码时      |
| `A0210` | 密码错误             | 登录/改密时旧密码错  |
| `A0230` | 登录过期             | access + refresh 均失效 |
| `A0240` | token 无效          | refresh token 校验失败  |
| `A0241` | 尝试超限             | 验证码重试次数超限    |
| `C0503` | 邮件发送失败         | SMTP 发送异常       |

handler 层统一使用 `errcode.Success(c, data)` 和
`errcode.Error(c, code, message)` 替代原有的 `model.Success` /
`model.BadRequest` 等函数。

### 3. 双 token 机制

**Access Token**：
- JWT，有效期 2 小时
- 随登录/刷新下发，客户端每次请求携带
- 过期时业务码为 `A0230`

**Refresh Token**：
- 不透明随机串（32 字节 hex），有效期 30 天
- 服务端存储哈希（SHA-256）到 `refresh_tokens` 表
- 每次刷新时旧 token 作废、下发新 refresh token（Rotation）

**`refresh_tokens` 表**：

| 字段       | 类型     | 说明                    |
|-----------|---------|------------------------|
| id        | INTEGER | 主键                    |
| token_hash| TEXT    | SHA-256 哈希，唯一索引     |
| user_id   | INTEGER | 关联 users.id            |
| expires_at| DATETIME| 过期时间                  |
| created_at| DATETIME| 创建时间                  |

**刷新流程**：`POST /api/auth/refresh` → 校验 refresh token →
作旧发新 → 返回新双 token。失败返回 `A0230`。

### 4. 账号体系

> **注意**：本节已被 [ADR-0002](0002-email-based-registration-and-account-model.md) 更迭。
> 当前主标识为 `email`，`username` 为展示用名（非唯一、注册时自动生成）。
> 以下为原决策记录，保留作历史参考。

- **主标识**：用户名（`username`），注册时必填、唯一
- **邮箱**：注册时必填（改为 `binding:"required,email"`），
  作为密码找回唯一通道；数据库加唯一索引
- **昵称**：`users` 表新增 `nickname` 字段，注册时默认 = username
- **头像**：`users` 表新增 `avatar` 字段，默认空串

### 5. 密码管理

> **注意**：「忘记密码」部分已被 ADR-0002 更迭，找回密码不再依赖 `username`，
> 完全走邮箱。以下 `forgot-password` 请求体已更新为当前实现。

**已登录改密**：`PUT /api/users/me/password`
- 请求体 `{old_password, new_password}`
- 服务端校验旧密码，通过后更新哈希

**忘记密码**：
1. `POST /api/auth/forgot-password/send-code` — 请求体
   `{email}`，校验账号存在后生成 6 位验证码
   （5 分钟有效，存内存 map），通过 SMTP 发送到邮箱
2. `POST /api/auth/forgot-password/reset` — 请求体
   `{email, code, new_password}`，校验验证码，通过后更新密码

### 6. 登录/注册响应载荷

登录和注册统一返回 `{token, refresh_token, user}`：

```json
{
  "code": "00000",
  "error": "",
  "data": {
    "token": "eyJ...",
    "refresh_token": "a1b2c3...",
    "user": {
      "id": 1,
      "username": "testuser",
      "nickname": "testuser",
      "email": "test@example.com",
      "avatar": ""
    }
  }
}
```

## 被拒方案

**正典 REST 状态码派**（HTTP 语义表达成败）：密码错误返回 401、参数错误返回
400。被拒原因：移动端 HTTP 客户端对非 2xx 直接抛异常，响应体中的业务错误
文案丢失，用户只能看到通用兜底文案。本项目为单一移动客户端 + 自维护小后端，
curl/网关监控的业务成败需求可忽略。

## 下游影响

### 需改动的文件

- `model/response.go` — Response 结构体字段类型 Int→String、message→error
- `model/errors.go` — 业务错误改为 errcode 常量
- `pkg/errcode/` — 新建，定义业务码常量 + 统一响应 helper
- `handler/*.go` — 所有 `model.Success`/`model.BadRequest` 等调用改为
  `errcode.Success`/`errcode.Error`
- `service/auth.go` — 登录返回 refresh token；注册返回完整用户信息 + 双 token
- `handler/auth.go` — 新增 refresh、logout、改密、发码、重置密码端点
- `model/user.go` — `RegisterRequest` email 改为 required；
  新增 `ModifyPwdRequest`、`SendCodeRequest`、`ResetPasswordRequest`；
  User 表加 `nickname` 字段
- `sql/init.sql` — users 表加 `nickname`、`avatar`；新增 `refresh_tokens` 表
- `pkg/jwt/` — access token 有效期从 72h 改为 2h
- `config.yaml` — 新增 SMTP 配置节

### API 路由变更

| 方法   | 路径                              | 说明           | 认证 |
|--------|----------------------------------|---------------|------|
| POST   | /api/auth/register               | 注册           | 否   |
| POST   | /api/auth/login                  | 登录           | 否   |
| POST   | /api/auth/refresh                | 刷新 token     | 否（用 refresh token） |
| POST   | /api/auth/logout                 | 登出           | 是   |
| PUT    | /api/users/me/password           | 已登录改密      | 是   |
| POST   | /api/auth/forgot-password/send-code | 发邮箱验证码  | 否   |
| POST   | /api/auth/forgot-password/reset  | 验证码重置密码   | 否   |
| GET    | /api/users/me                    | 获取用户信息    | 是   |
| PUT    | /api/users/me                    | 更新用户信息    | 是   |

### 验收标准

1. `go test ./...` 全量通过
2. curl 走通注册→登录→刷新→改密→忘记密码全链路
3. 所有业务端点 HTTP 状态码均为 200
4. `refresh_tokens` 表正确轮换
5. SMTP 发码→验码→重置密码链路畅通
