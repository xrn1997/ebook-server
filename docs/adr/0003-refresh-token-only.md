# refresh 仅凭证、登录才建身份（token 与身份解耦）

将「认证凭证」与「用户身份」在服务端认证模块解耦：`POST /api/auth/refresh`
只返回纯双 token（`{token, refresh_token}`），**不再回填用户资料**；
登录/注册仍返回完整 `user`。身份信息以登录/注册为唯一下发点。

## 动机

* **轮换语义不该携带 profile**：OAuth2/OIDC 惯例下，refresh 响应只含新 token，不含用户资料。
  此前登录与刷新共用 `issueTokens`，把完整 `user` 一并下放，既过度序列化，也诱使
  客户端用刷新响应重建/覆写本地身份。

* **消除客户端身份覆写隐患**：若客户端以刷新响应重建会话，一旦刷新响应不含 user，
  就会用空身份抹掉已有昵称/头像/uid。客户端据 ADR-0011 已改用「仅轮换凭证」操作，
  服务端须把契约收紧到与之匹配，避免两端语义错位。

* **单一身份来源**：身份只在登录/注册时下发并持久化；刷新只管续期凭证，职责单一。

## 决策

1. **新增** **`model.TokenPayload`**（`{token, refresh_token}`，无 `user`），刷新专用。
2. **`Refresh`** **返回** **`*model.TokenPayload`**：复用 `issueTokenPayload(uid, username)`
   签发纯凭证；user 仅用于构造 JWT（uid/username），不进入刷新响应。
3. **登录/注册保持** **`TokenPair`（含 user）**：`issueTokens` 委托
   `issueTokenPayload` 后附加 `User`，两者共用签发逻辑、不重复。
4. **不动 access/refresh 的时效与轮换语义**：access 2h、refresh 30 天、刷新即作废旧值。

## 实现要点

`service/auth.go`：

```go
func (s *AuthService) Refresh(refreshToken string) (*model.TokenPayload, error) {
    record, err := s.tokenRepo.FindByHash(sha256Hex(refreshToken))
    if err != nil {
        if repository.IsRecordNotFound(err) {
            return nil, model.ErrLoginExpired
        }
        return nil, err
    }
    user, err := s.userRepo.FindByUID(record.UserID)
    if err != nil {
        return nil, err
    }
    // Rotation：作废旧 token，下发新双 token（仅凭证，不含用户资料）
    if err := s.tokenRepo.DeleteByID(record.ID); err != nil {
        return nil, err
    }
    return s.issueTokenPayload(user.UID, user.Username)
}

// issueTokenPayload 签发双 token（纯凭证）——登录复用其凭证部分、刷新直接使用
func (s *AuthService) issueTokenPayload(uid uint, username string) (*model.TokenPayload, error) {
    accessToken, err := jwt.GenerateToken(uid, username)
    if err != nil {
        return nil, err
    }
    refreshToken, err := s.newRefreshToken(uid)
    if err != nil {
        return nil, err
    }
    return &model.TokenPayload{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

func (s *AuthService) issueTokens(user *model.User) (*model.TokenPair, error) {
    payload, err := s.issueTokenPayload(user.UID, user.Username)
    if err != nil {
        return nil, err
    }
    return &model.TokenPair{
        AccessToken:  payload.AccessToken,
        RefreshToken: payload.RefreshToken,
        User:         *user,
    }, nil
}
```

`model/user.go` 追加：

```go
// TokenPayload 纯凭证载荷（刷新端点用，不含用户资料）
type TokenPayload struct {
    AccessToken  string `json:"token"`
    RefreshToken string `json:"refresh_token"`
}
```

`handler/auth.go` 无需改动（`Refresh` 已 `errcode.Success(c, pair)`，自动命中新返回类型）。

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

