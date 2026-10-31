# 响应信封重构 + 认证模型升级

Status: accepted
Superseded in part by: 0002（账号体系、密码管理）

将响应信封从 `{code: Int, message, data}` 改为 `{code: String, error, data}`，HTTP 状态码恒为 200；同时引入双 token 机制、用户名+邮箱账号体系、密码找回流程。

## 背景

改造前的 `model/response.go` 定义的是 `Code int` 与 `Message string`，业务错误（如密码错误）通过 HTTP 状态码表达：`BadRequest` 返回 400，`Unauthorized` 返回 401，`handler/auth.go` 的 `Login` 失败时调用 `model.Unauthorized(c, err.Error())`。

认证侧是单 token（JWT，72 小时过期），登录返回 `{token, username, email}`；没有 refresh token，token 过期后用户必须重新登录。密码用 bcrypt 哈希存储，注册邮箱可选。`RegisterRequest` 与 `LoginRequest` 均以 `username` 为主标识，`users` 表的 `email` 字段存在但非必填。

### 信封重构

移动端 HTTP 客户端（Retrofit）对非 2xx 响应直接抛异常，响应体无法到达业务解析层。具体问题：

1. 后端返回 401 + `{code: 401, message: "密码错误"}` → Retrofit 抛 `HttpException` → 响应体丢失 → 用户看到的是兜底文案"操作未授权"而非"密码错误"
2. 业务错误文案无法直达用户 Toast，丢失了服务端精心构造的错误信息

改为 HTTP 200 + 业务码后，所有响应都能到达业务解析层，错误文案可直达用户。

### 双 token

单 token 有效期 72 小时，过期即强制重新登录。对于日常使用的阅读 App，用户期望"打开就能用"。双 token（access 短效 + refresh 长效）可在 refresh 有效期内无感续期。

### 邮箱必填

密码找回需要邮件通道。当时邮箱可选导致部分用户无法找回密码。

## 决策

### 1. 响应信封

```go
type Response struct {
    Code  string      `json:"code"`  // 五位业务码，"00000" 表示成功
    Error string      `json:"error"` // 业务错误文案（成功时为空串）
    Data  interface{} `json:"data,omitempty"`
}
```

HTTP 状态码规则：
- **200**：业务层有响应（无论成败），所有业务端点一律返回 200
- **非 200**：仅用于网关故障、进程崩溃等传输层异常（本项目为单进程，正常运行不会出现非 200）

### 2. 业务错误码

业务码住在 `backend/pkg/errcode`，是五位字符串。`00000` 表示成功（常量 `model.SuccessCode`，`errcode.SuccessCode` 是它的别名）。有产生方的业务码：

| 码      | 含义               | 使用场景           |
|---------|-------------------|-------------------|
| `A0112` | 邮箱已被注册         | 注册时邮箱冲突       |
| `A0113` | 登录主标识不可变更     | 改资料时传了与当前值不同的 email（ADR-0004） |
| `A0120` | 密码格式不合规       | 密码超过 bcrypt 的 72 字节上限（`service.Credentials` 判定；binding 的 max 按字符计，多字节密码可能先通过校验） |
| `A0132` | 验证码错误           | 验证码不匹配或已过期   |
| `A0201` | 账户不存在           | **仅限已认证上下文**（如 `GET /api/users/me`）；登录/找回密码不得使用（ADR-0006） |
| `A0210` | 密码错误             | 登录/改密时旧密码错；**登录时账号不存在也归于此码**（ADR-0006） |
| `A0230` | 登录过期             | access 无效或 refresh 校验失败 |
| `A0241` | 尝试超限             | 验证码重试次数超限、发码频率超限（ADR-0002 §4） |
| `A0242` | 账号已锁定           | 保留码（内部锁定状态）；公开登录按 ADR-0006 统一回 `A0210`，不对外返回 |
| `A0303` | 无权删除评论         | 删除他人评论（ADR-0011） |
| `A0304` | 评论不存在           | 评论未找到（ADR-0011） |
| `A0305` | 新旧聚合键相同         | 改键时新旧键一致（ADR-0014） |
| `A0400` | 请求参数错误         | gin binding 校验失败，含所有格式类校验 |
| `A0403` | 无权限              | 后台认证失败（管理端统一用此码，后台前端据此重登） |
| `C0500` | 服务器内部错误       | 未归类异常          |
| `C0503` | 邮件发送失败         | SMTP 发送异常       |

`A0110`（用户名格式不合规）、`A0111`（用户名已存在）、`A0153`（邮箱格式错误）、`A0240`（token 无效）、`A0404`（资源不存在）已没有产生方：用户名不再作登录主标识（ADR-0002）、格式类校验统一由 gin binding 承接并落 `A0400`、token 无效统一表达为 `A0230`、资源未找到由各自的领域码表达（评论见 `A0304`）。`A0301`（评论内容为空）与 `A0302`（章节非法）是预留码，暂不启用。常量仍留在 `errcode` 里——它们是对外词表的一部分，删掉会让「这个码以前叫什么」无从查证。

`A0201` 的适用范围被 ADR-0006 收窄——登录与找回密码不再暴露账号是否存在。

handler 层统一使用 `errcode.Success(c, data)` 与 `errcode.Error(c, code, message)`。

### 3. 双 token 机制

**Access Token**：
- JWT，有效期 2 小时
- 随登录/刷新下发，客户端每次请求携带
- 过期时业务码为 `A0230`

**Refresh Token**：
- 不透明随机串（32 字节 hex），有效期 30 天
- 服务端存储哈希（SHA-256）到 `refresh_tokens` 表
- 每次刷新时旧 token 作废、下发新 refresh token（Rotation）

**刷新流程**：`POST /api/auth/refresh` → 校验 refresh token → 作旧发新 → 返回新双 token。失败返回 `A0230`。

### 4. 账号体系

> **注意**：本节已被 [ADR-0002](0002-email-based-registration-and-account-model.md) 更迭。
> 当前主标识为 `email`，`username` 为展示用名（非唯一、注册时自动生成）。
> 以下为原决策记录，保留作历史参考。

- **主标识**：用户名（`username`），注册时必填、唯一
- **邮箱**：注册时必填（改为 `binding:"required,email"`），作为密码找回唯一通道；数据库加唯一索引
- **昵称**：`users` 表新增 `nickname` 字段，注册时默认 = username
- **头像**：`users` 表新增 `avatar` 字段，默认空串

### 5. 密码管理

> **注意**：「忘记密码」部分已被 ADR-0002 更迭，找回密码不再依赖 `username`，
> 完全走邮箱。以下 `forgot-password` 请求体已更新为当前实现。

**已登录改密**：`PUT /api/users/me/password`
- 请求体 `{old_password, new_password}`
- 服务端校验旧密码，通过后更新哈希

**忘记密码**：
1. `POST /api/auth/forgot-password/send-code` — 请求体 `{email}`，校验账号存在后生成 6 位验证码（5 分钟有效，存内存 map），通过 SMTP 发送到邮箱
2. `POST /api/auth/forgot-password/reset` — 请求体 `{email, code, new_password}`，校验验证码，通过后更新密码

### 6. 登录/注册响应载荷

**登录**（`POST /api/auth/login`）返回 `model.TokenPair`：`{token, refresh_token, user}`。

**注册**（`POST /api/auth/register`）只回 `data: null`，不发 token（ADR-0002/0003）：
建号即激活，取得会话的唯一路径是登录。

```json
{
  "code": "00000",
  "error": "",
  "data": {
    "token": "eyJ...",
    "refresh_token": "a1b2c3...",
    "user": {
      "uid": 1,
      "username": "user_1a2b3c4d",
      "nickname": "user_1a2b3c4d",
      "email": "test@example.com",
      "avatar": ""
    }
  }
}
```

## 被拒方案

**正典 REST 状态码派**（HTTP 语义表达成败）：密码错误返回 401、参数错误返回 400。被拒原因：移动端 HTTP 客户端对非 2xx 直接抛异常，响应体中的业务错误文案丢失，用户只能看到通用兜底文案。本项目为单一移动客户端 + 自维护小后端，curl/网关监控的业务成败需求可忽略。

## 边界

- 当前端点清单与鉴权标记住 `README.md` 与 `backend/router/router.go`，本文不重复。
- 端点级业务码覆盖（如登录为防枚举把「账号不存在」映射成 `A0210`）写在调用点，见 `backend/pkg/errcode` 的 `Map`。

锚点：`backend/pkg/errcode` 的 `Respond`；`backend/model/response.go` 的 `SuccessCode`；`backend/middleware/auth.go`；`backend/router/router_test.go`。
