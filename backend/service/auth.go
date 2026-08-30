package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/jwt"
	"ebook-server/repository"

	"golang.org/x/crypto/bcrypt"
)

// RefreshTokenTTL refresh token 有效期（30 天）
const RefreshTokenTTL = 30 * 24 * time.Hour

// 登录防暴力锁定参数（ADR-0002）
const (
	maxLoginAttempts  = 5
	loginLockDuration = 15 * time.Minute
)

// AuthService 认证业务服务：注册、登录、token 签发与轮换、密码管理。
//
// 数据访问依赖以 Store 接口注入（ADR-0007）；验证码存储仍以具体类型注入（校验用），
// 下发验证码走 VerificationCodeSender（ADR-0008）。
// 账号注销与数据导出见 AccountService。
type AuthService struct {
	users  UserStore
	tokens TokenStore
	codes  *code.Store             // 校验验证码（注册/找回）
	sender *VerificationCodeSender // 下发验证码（限流+存码+发信）
}

// NewAuthService 创建认证服务实例。
func NewAuthService(users UserStore, tokens TokenStore, codes *code.Store, sender *VerificationCodeSender) *AuthService {
	return &AuthService{
		users:  users,
		tokens: tokens,
		codes:  codes,
		sender: sender,
	}
}

// SendCode 发送注册验证码到邮箱（不限账号是否已存在，注册前调用）
func (s *AuthService) SendCode(email string) error {
	return s.sender.Send(FlowRegister, email)
}

// Register 注册：校验验证码 + 密码，激活建号，不发 token
func (s *AuthService) Register(req *model.RegisterRequest) (*model.User, error) {
	// 邮箱唯一
	exists, err := s.users.ExistsByEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, model.ErrEmailExists
	}

	// 校验注册验证码
	if s.codes.Verify(FlowRegister.prefix()+req.Email, req.Code) != code.ResultOK {
		return nil, model.ErrCodeInvalid
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	username := s.generateUsername()
	user := &model.User{
		Email:    req.Email,
		Password: string(hashed),
		Username: username,
		Nickname: username,
	}

	if err := s.users.Create(user); err != nil {
		return nil, err
	}

	// 注册完成即已激活；不发 token（token 仅由 login 下发）
	return user, nil
}

// Login 登录：校验账号 + 密码 + 锁定，签发双 token
func (s *AuthService) Login(req *model.LoginRequest) (*model.TokenPair, error) {
	user, err := s.users.FindByEmail(req.Email)
	if err != nil {
		if IsRecordNotFound(err) {
			return nil, model.ErrAccountNotFound
		}
		return nil, err
	}

	// 锁定期间即使密码正确也拒绝
	if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		return nil, model.ErrAccountLocked
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return s.recordLoginFailure(user)
	}
	return s.recordLoginSuccess(user)
}

// recordLoginFailure 登录失败累加，达到上限则锁定
func (s *AuthService) recordLoginFailure(user *model.User) (*model.TokenPair, error) {
	user.LoginAttempts++
	now := time.Now()
	if user.LoginAttempts >= maxLoginAttempts {
		until := now.Add(loginLockDuration)
		user.LockedUntil = &until
		user.LoginAttempts = 0
		_ = s.users.Update(user)
		return nil, model.ErrAttemptTooMany
	}
	_ = s.users.Update(user)
	return nil, model.ErrPasswordWrong
}

// recordLoginSuccess 登录成功，清零失败计数并签发双 token
func (s *AuthService) recordLoginSuccess(user *model.User) (*model.TokenPair, error) {
	if user.LoginAttempts != 0 || user.LockedUntil != nil {
		user.LoginAttempts = 0
		user.LockedUntil = nil
		if err := s.users.Update(user); err != nil {
			return nil, err
		}
	}
	return s.issueTokens(user)
}

// Refresh 刷新 token（ADR-0003）：校验旧 refresh token 后作废，仅下发纯凭证（不含用户资料）
func (s *AuthService) Refresh(refreshToken string) (*model.TokenPayload, error) {
	record, err := s.tokens.FindByHash(sha256Hex(refreshToken))
	if err != nil {
		if IsRecordNotFound(err) {
			return nil, model.ErrLoginExpired
		}
		return nil, err
	}

	user, err := s.users.FindByUID(record.UserID)
	if err != nil {
		return nil, err
	}

	// Rotation：作废旧 token，下发新双 token（仅凭证，不含用户资料）
	if err := s.tokens.DeleteByID(record.ID); err != nil {
		return nil, err
	}
	return s.issueTokenPayload(user.UID, user.Username)
}

// Logout 登出，作废该用户的所有 refresh token
func (s *AuthService) Logout(uid uint) error {
	return s.tokens.DeleteByUserID(uid)
}

// ChangePassword 已登录修改密码，成功后使该用户全部 token 失效
func (s *AuthService) ChangePassword(uid uint, oldPassword, newPassword string) error {
	user, err := s.users.FindByUID(uid)
	if err != nil {
		if IsRecordNotFound(err) {
			return model.ErrUserNotFound
		}
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return model.ErrPasswordWrong
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hashed)
	if err := s.users.Update(user); err != nil {
		return err
	}

	// 改密后旧 access/refresh 一律失效
	return s.tokens.DeleteByUserID(uid)
}

// SendForgotCode 忘记密码发送验证码（仅账号存在时真实发送，避免枚举）
func (s *AuthService) SendForgotCode(email string) error {
	// 防枚举（ADR-0006）：账号不存在也返回成功、不发送。这枚存在性检查留在流程调用方，
	// sender 对账号无感——枚举保护是找回流程的策略，不是「发码」本身的职责。
	if _, err := s.users.FindByEmail(email); err != nil {
		if IsRecordNotFound(err) {
			return nil // 账号不存在也返回成功，不暴露枚举
		}
		return err
	}
	return s.sender.Send(FlowForgot, email)
}

// ResetPassword 验证码重置密码（纯邮箱）
func (s *AuthService) ResetPassword(email, codeVal, newPassword string) error {
	switch s.codes.Verify(FlowForgot.prefix()+email, codeVal) {
	case code.ResultOK:
		// 校验通过
	case code.ResultTooManyAttempts:
		return model.ErrAttemptTooMany
	default:
		return model.ErrCodeInvalid
	}

	user, err := s.users.FindByEmail(email)
	if err != nil {
		if IsRecordNotFound(err) {
			return model.ErrAccountNotFound
		}
		return err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hashed)
	if err := s.users.Update(user); err != nil {
		return err
	}

	return s.tokens.DeleteByUserID(user.UID)
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
func (s *AuthService) issueTokenPayload(uid uint, username string) (*model.TokenPayload, error) {
	accessToken, err := jwt.GenerateToken(uid, username)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.newRefreshToken(uid)
	if err != nil {
		return nil, err
	}

	return &model.TokenPayload{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// issueTokens 生成双 token 并落库 refresh token（登录/注册专用，含用户资料）
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

// newRefreshToken 生成 32 字节 hex 随机串并落库其 SHA-256 哈希
func (s *AuthService) newRefreshToken(uid uint) (string, error) {
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
	if err := s.tokens.Create(record); err != nil {
		return "", err
	}
	return raw, nil
}

// sha256Hex 计算字符串的 SHA-256 十六进制摘要
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// IsRecordNotFound 转发 repository 的 gorm 哨兵判断。
//
// 保留此薄转发以减小本次改造的扩散面；service 层对 gorm 的依赖经此唯一窄口
// （ADR-0007「被拒方案」：错误翻译留待需要时单独立项）。
func IsRecordNotFound(err error) bool {
	return repository.IsRecordNotFound(err)
}
