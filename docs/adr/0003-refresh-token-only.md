# refresh 仅凭证、登录才建身份（token 与身份解耦）

Status: accepted

将「认证凭证」与「用户身份」在服务端认证模块解耦：`POST /api/auth/refresh`
只返回纯双 token（`{token, refresh_token}`），**不再回填用户资料**；
只有登录返回完整 `user`，注册只回成功、不发 token。身份信息以登录为唯一下发点。

## 动机

* **轮换语义不该携带 profile**：OAuth2/OIDC 惯例下，refresh 响应只含新 token，不含用户资料。
  此前登录与刷新共用 `issueTokens`，把完整 `user` 一并下放，既过度序列化，也诱使
  客户端用刷新响应重建/覆写本地身份。

* **消除客户端身份覆写隐患**：若客户端以刷新响应重建会话，一旦刷新响应不含 user，
  就会用空身份抹掉已有昵称/头像/uid。客户端据 ADR-0011 已改用「仅轮换凭证」操作，
  服务端须把契约收紧到与之匹配，避免两端语义错位。

* **单一身份来源**：身份只在登录时下发并持久化；刷新只管续期凭证，职责单一。

## 决策

1. **新增** **`model.TokenPayload`**（`{token, refresh_token}`，无 `user`），刷新专用。
2. **`Refresh`** **返回** **`*model.TokenPayload`**：复用 `issueTokenPayload(uid, username)`
   签发纯凭证；user 仅用于构造 JWT（uid/username），不进入刷新响应。
3. **登录保持** **`TokenPair`（含 user）**：`issueTokens` 委托
   `issueTokenPayload` 后附加 `User`，两者共用签发逻辑、不重复；注册不发 token。
4. **不动 access/refresh 的时效与轮换语义**：access 默认 2h（`jwt.expire_min` 可配）、
   refresh 30 天（`service.RefreshTokenTTL`）、刷新即作废旧值。

   **撤销只作用于 refresh**：改密与登出撤销该用户的全部 refresh token，已签发的 access
   token 无状态、活到自身过期为止（默认 2h）。这是无状态 JWT 换来的取舍——服务端不留
   access 会话表，就不存在「立刻作废某一枚 access」这条路。客户端侧 access 只驻内存
   （android-ebook ADR-0011），重启即失；需要即时断权的场景（注销）由匿名化把 uid
   对应的资料清空来兜住，见 ADR-0005。

## 实现要点

- `model.TokenPayload`（`backend/model/user.go`）：纯凭证载荷 `{token, refresh_token}`，无 `user`。
- `service/auth.go`：`Refresh` 返回 `*model.TokenPayload`——先按哈希找旧 token，条件删除成功才签发
  （没删到行说明已被并发刷新消费，回 `model.ErrLoginExpired`/A0230），再经 `issueTokenPayload`
  签发纯凭证；`issueTokens` 委托 `issueTokenPayload` 后附加 `User`，登录仍返回含 `user` 的 `TokenPair`。
- `handler/auth.go`：`Refresh` 沿用 `errcode.Success(c, pair)`，响应体随返回类型自动变为 `{token, refresh_token}`。

细节以代码与测试为准；本节只保留决定中不可从代码读出的语义（轮换失败即拒绝、身份仅登录下发）。

## 被拒方案

* **保留 refresh 回 user**：违反刷新惯例，且持续诱导客户端覆写本地身份（见动机）。

* **刷新也用** **`TokenPair`** **但塞空 user**：`json:"user,omitempty"` 对零值 struct 不生效，
  会序列化出空 `user` 对象，契约不干净；专用 `TokenPayload` 更明确。

## 下游影响

* `model/user.go`：新增 `TokenPayload`。

* `service/auth.go`：拆分 `issueTokenPayload`/`issueTokens`，`Refresh` 返回 `TokenPayload`。

* `handler/auth.go`：`Refresh` 响应体变为 `{token, refresh_token}`。

* 服务端测试：`TestAuthService_Refresh_Rotation` 兼容（仅校验 `RefreshToken`）；
  如存在断言刷新响应含 `user` 的测试需相应删除/更新。

* 客户端：配合 android-ebook ADR-0011（refresh 仅凭证、rotateCredentials、access 只驻内存）。

## 验收

1. `go test ./...` 全量通过。
2. curl 登录返回 `user`；`/api/auth/refresh` 返回体**不含** `user` 键。
3. 登录→刷新→登出全链路 token 正确轮换。

锚点：`backend/service/auth.go` 的 `Refresh`；`backend/repository/refresh_token.go`。

