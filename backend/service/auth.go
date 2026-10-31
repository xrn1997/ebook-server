package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/jwt"
)

// RefreshTokenTTL refresh token 有效期（30 天）
const RefreshTokenTTL = 30 * 24 * time.Hour

// AuthService 认证业务服务：注册、登录、token 签发与轮换、密码管理。
//
// 数据访问依赖以 Store 接口注入（ADR-0007）；验证码的下发与校验都走
// VerificationCodes（ADR-0008），本模块不自建键、不解释校验结果的枚举。
// access token 的签发与解析由注入的 jwt.TokenBox 负责（密钥与有效期是构造时给的，
// 不是去读包级配置）。账号注销与数据导出见 AccountService。
type AuthService struct {
	users       UserStore
	tokens      TokenStore
	codes       *VerificationCodes
	access      *jwt.TokenBox
	credentials *Credentials
}

// NewAuthService 创建认证服务实例。access 是用户侧 token 单元；
// 密码哈希、失败计数与会话撤销由注入的 Credentials 承担。
func NewAuthService(users UserStore, tokens TokenStore, codes *VerificationCodes, access *jwt.TokenBox, credentials *Credentials) *AuthService {
	return &AuthService{
		users:       users,
		tokens:      tokens,
		codes:       codes,
		access:      access,
		credentials: credentials,
	}
}

// SendCode 发送注册验证码到邮箱（不限账号是否已存在，注册前调用）
func (s *AuthService) SendCode(ctx context.Context, email string) error {
	return s.codes.Send(ctx, FlowRegister, email)
}

// Register 注册：校验验证码 + 密码，激活建号，不发 token
func (s *AuthService) Register(ctx context.Context, req *model.RegisterRequest) (*model.User, error) {
	// 先校验验证码，再检查邮箱是否已注册（ADR-0006）。
	// 反过来会让注册端点变成无需验证码的邮箱存在性探针：同一个错码，已注册邮箱回
	// A0112、未注册回 A0132。先验证码则只有能收到该邮箱验证码的人才会看到 A0112，
	// 那已经证明他控制该邮箱。
	if err := s.codes.Confirm(ctx, FlowRegister, req.Email, req.Code); err != nil {
		return nil, err
	}

	exists, err := s.users.ExistsByEmail(ctx, req.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, model.ErrEmailExists
	}

	username := s.generateUsername()
	user := &model.User{
		Email:    req.Email,
		Username: username,
		Nickname: username,
	}

	if err := s.credentials.CreateWithPassword(ctx, user, req.Password); err != nil {
		return nil, err
	}

	// 注册完成即已激活；不发 token（token 仅由 login 下发）
	return user, nil
}

// Login 登录：校验账号 + 密码 + 锁定，签发双 token
func (s *AuthService) Login(ctx context.Context, req *model.LoginRequest) (*model.TokenPair, error) {
	// 账号不存在时 repository 已给出 model.ErrAccountNotFound，这里不再判一次再翻译一次；
	// 未认证端点不能透出「账号不存在」，改码在 handler 用 errcode.Map 声明（ADR-0006）。
	user, err := s.users.FindByEmail(ctx, req.Email)
	switch {
	case err == nil:
	case errors.Is(err, model.ErrAccountNotFound):
		user = nil // 不提前返回：下面照样花掉一次口令比对的代价
	default:
		return nil, err
	}

	// 比对放在所有分支之前，且只做一次：省掉它会让「邮箱没注册」和「账号被锁」
	// 各自露出一个可测的耗时档（ADR-0006 的时序面）。
	passwordOK := s.credentials.Verify(user, req.Password)

	if user == nil {
		return nil, model.ErrAccountNotFound
	}
	// 锁定期间即使密码正确也拒绝
	if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		return nil, model.ErrAccountLocked
	}
	if !passwordOK {
		return s.recordLoginFailure(ctx, user)
	}
	return s.recordLoginSuccess(ctx, user)
}

// recordLoginFailure 登录失败累加，达到上限则锁定
func (s *AuthService) recordLoginFailure(ctx context.Context, user *model.User) (*model.TokenPair, error) {
	locked, err := s.credentials.RecordFailedLogin(ctx, user.UID)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, model.ErrAttemptTooMany
	}
	return nil, model.ErrPasswordWrong
}

// recordLoginSuccess 登录成功，清零失败计数并签发双 token
//
// 无条件清零：FindByEmail 读到的计数可能已过期，数据库才是事实来源；
// 这条 UPDATE 很便宜，换来的是「并发失败计数不会在成功登录后残留」。
func (s *AuthService) recordLoginSuccess(ctx context.Context, user *model.User) (*model.TokenPair, error) {
	if err := s.credentials.ClearFailures(ctx, user.UID); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, user)
}

// Refresh 刷新 token（ADR-0003）：校验旧 refresh token 后作废，仅下发纯凭证（不含用户资料）
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*model.TokenPayload, error) {
	record, err := s.tokens.FindByHash(ctx, sha256Hex(refreshToken))
	if err != nil {
		return nil, err // 令牌不存在或已过期，repository 一并给 model.ErrLoginExpired
	}

	user, err := s.users.FindByUID(ctx, record.UserID)
	if err != nil {
		return nil, err // 令牌指向的账号已被硬删（注销只匿名化，见 ADR-0005）→ A0201
	}
	// 匿名化注销的账号一律拒绝续期（→ A0201）：注销与并发刷新的竞态可能让轮换
	// 写入的令牌漏过 DeleteByUserID，续期前的这道状态检查保证它最多存活到自身
	// 过期、绝不轮换——否则一条会话能在账号注销后无限续期（isAnonymized）。
	if isAnonymized(user) {
		return nil, model.ErrAccountNotFound
	}

	// Rotation：条件作废旧 token，只有真正删到行的一方才签发新令牌。
	// 并发刷新同一枚令牌时，后来者在这里被当作「令牌已失效」拒绝。
	consumed, err := s.tokens.DeleteByID(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, model.ErrLoginExpired
	}
	return s.issueTokenPayload(ctx, user.UID, user.Username)
}

// Logout 登出：作废该用户的全部 refresh token。
//
// access token 是无状态的，这里撤不掉它，它活到自身过期（ADR-0003 的取舍）。
func (s *AuthService) Logout(ctx context.Context, uid uint) error {
	return s.tokens.DeleteByUserID(ctx, uid)
}

// ChangePassword 已登录修改密码；成功后撤销该用户的全部 refresh token
// （access 仍会活到自身过期，同上——别把这句读成「立即断掉所有会话」）。
func (s *AuthService) ChangePassword(ctx context.Context, uid uint, oldPassword, newPassword string) error {
	user, err := s.users.FindByUID(ctx, uid)
	if err != nil {
		return err
	}

	if !s.credentials.CheckPassword(user.Password, oldPassword) {
		return model.ErrPasswordWrong
	}

	// SetPassword 内部先撤销 refresh token、再写新哈希（fail-closed）。
	return s.credentials.SetPassword(ctx, uid, newPassword)
}

// SendForgotCode 忘记密码发送验证码（仅账号存在时真实发送，避免枚举）
func (s *AuthService) SendForgotCode(ctx context.Context, email string) error {
	// 防枚举（ADR-0006）：账号不存在也返回成功、不发送。这枚存在性检查留在流程调用方，
	// sender 对账号无感——枚举保护是找回流程的策略，不是「发码」本身的职责。
	if _, err := s.users.FindByEmail(ctx, email); err != nil {
		if errors.Is(err, model.ErrAccountNotFound) {
			return nil // 账号不存在也返回成功，不暴露枚举
		}
		return err
	}
	return s.codes.Send(ctx, FlowForgot, email)
}

// ResetPassword 验证码重置密码（纯邮箱）
func (s *AuthService) ResetPassword(ctx context.Context, email, codeVal, newPassword string) error {
	if err := s.codes.Confirm(ctx, FlowForgot, email, codeVal); err != nil {
		return err
	}

	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return err
	}

	return s.credentials.SetPassword(ctx, user.UID, newPassword)
}

// generateUsername 生成随机用户名占位（可后改）
func (s *AuthService) generateUsername() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "user00000000"
	}
	return "user_" + hex.EncodeToString(buf)
}

// issueTokenPayload 签发双 token（纯凭证，不含用户资料）——登录复用其凭证部分、刷新直接使用
func (s *AuthService) issueTokenPayload(ctx context.Context, uid uint, username string) (*model.TokenPayload, error) {
	accessToken, err := s.access.IssueUser(uid, username)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.newRefreshToken(ctx, uid)
	if err != nil {
		return nil, err
	}

	return &model.TokenPayload{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// issueTokens 生成双 token 并落库 refresh token（登录/注册专用，含用户资料）
func (s *AuthService) issueTokens(ctx context.Context, user *model.User) (*model.TokenPair, error) {
	payload, err := s.issueTokenPayload(ctx, user.UID, user.Username)
	if err != nil {
		return nil, err
	}

	return &model.TokenPair{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		User:         *user,
	}, nil
}

// newRefreshToken 生成 32 字节 hex 随机串并落库其 SHA-256 哈希
func (s *AuthService) newRefreshToken(ctx context.Context, uid uint) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	raw := hex.EncodeToString(buf)

	record := &model.RefreshToken{
		TokenHash: sha256Hex(raw),
		UserID:    uid,
		ExpiresAt: time.Now().Add(RefreshTokenTTL),
	}
	if err := s.tokens.Create(ctx, record); err != nil {
		return "", err
	}
	return raw, nil
}

// sha256Hex 计算字符串的 SHA-256 十六进制摘要
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
