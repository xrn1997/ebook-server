package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/jwt"
	"ebook-server/pkg/mail"
	"ebook-server/pkg/ratelimit"
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

// 发码限流参数（ADR-0002）
var (
	sendCodeMinute = ratelimit.New(1, time.Minute) // 每分钟至多 1 次
	sendCodeHour   = ratelimit.New(5, time.Hour)   // 每小时至多 5 次
)

// 验证码存储 key 命名空间，隔离「注册」与「找回密码」两套流程
const (
	codeKeyRegister = "reg:"
	codeKeyForgot   = "forgot:"
)

type AuthService struct {
	userRepo  *repository.UserRepository
	tokenRepo *repository.RefreshTokenRepository
}

func NewAuthService() *AuthService {
	return &AuthService{
		userRepo:  repository.NewUserRepository(),
		tokenRepo: repository.NewRefreshTokenRepository(),
	}
}

// SendCode 发送注册验证码到邮箱（不限账号是否已存在，注册前调用）
func (s *AuthService) SendCode(email string) error {
	if err := s.allowSendCode(asKey(email)); err != nil {
		return err
	}
	codeVal := code.Default().Save(codeKeyRegister + email)
	return mail.SendCode(email, codeVal)
}

// Register 注册：校验验证码 + 密码，激活建号，不发 token
func (s *AuthService) Register(req *model.RegisterRequest) (*model.User, error) {
	// 邮箱唯一
	exists, err := s.userRepo.ExistsByEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, model.ErrEmailExists
	}

	// 校验注册验证码
	if code.Default().Verify(codeKeyRegister+req.Email, req.Code) != code.ResultOK {
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

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	// 注册完成即已激活；不发 token（token 仅由 login 下发）
	return user, nil
}

// Login 登录：校验账号 + 密码 + 锁定，签发双 token
func (s *AuthService) Login(req *model.LoginRequest) (*model.TokenPair, error) {
	user, err := s.userRepo.FindByEmail(req.Email)
	if err != nil {
		if repository.IsRecordNotFound(err) {
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
		_ = s.userRepo.Update(user)
		return nil, model.ErrAttemptTooMany
	}
	_ = s.userRepo.Update(user)
	return nil, model.ErrPasswordWrong
}

// recordLoginSuccess 登录成功，清零失败计数并签发双 token
func (s *AuthService) recordLoginSuccess(user *model.User) (*model.TokenPair, error) {
	if user.LoginAttempts != 0 || user.LockedUntil != nil {
		user.LoginAttempts = 0
		user.LockedUntil = nil
		if err := s.userRepo.Update(user); err != nil {
			return nil, err
		}
	}
	return s.issueTokens(user)
}

// Refresh 刷新 token：校验旧 refresh token 后作废并下放新双 token
func (s *AuthService) Refresh(refreshToken string) (*model.TokenPair, error) {
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

	// Rotation：作废旧 token，下发新双 token
	if err := s.tokenRepo.DeleteByID(record.ID); err != nil {
		return nil, err
	}

	return s.issueTokens(user)
}

// Logout 登出，作废该用户的所有 refresh token
func (s *AuthService) Logout(uid uint) error {
	return s.tokenRepo.DeleteByUserID(uid)
}

// ChangePassword 已登录修改密码，成功后使该用户全部 token 失效
func (s *AuthService) ChangePassword(uid uint, oldPassword, newPassword string) error {
	user, err := s.userRepo.FindByUID(uid)
	if err != nil {
		if repository.IsRecordNotFound(err) {
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
	if err := s.userRepo.Update(user); err != nil {
		return err
	}

	// 改密后旧 access/refresh 一律失效
	return s.tokenRepo.DeleteByUserID(uid)
}

// SendForgotCode 忘记密码发送验证码（仅账号存在时真实发送，避免枚举）
func (s *AuthService) SendForgotCode(email string) error {
	if err := s.allowSendCode(asKey(email)); err != nil {
		return err
	}
	if _, err := s.userRepo.FindByEmail(email); err != nil {
		if repository.IsRecordNotFound(err) {
			return nil // 账号不存在也返回成功，不暴露枚举
		}
		return err
	}
	codeVal := code.Default().Save(codeKeyForgot + email)
	return mail.SendCode(email, codeVal)
}

// ResetPassword 验证码重置密码（纯邮箱）
func (s *AuthService) ResetPassword(email, codeVal, newPassword string) error {
	switch code.Default().Verify(codeKeyForgot+email, codeVal) {
	case code.ResultOK:
		// 校验通过
	case code.ResultTooManyAttempts:
		return model.ErrAttemptTooMany
	default:
		return model.ErrCodeInvalid
	}

	user, err := s.userRepo.FindByEmail(email)
	if err != nil {
		if repository.IsRecordNotFound(err) {
			return model.ErrAccountNotFound
		}
		return err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hashed)
	if err := s.userRepo.Update(user); err != nil {
		return err
	}

	return s.tokenRepo.DeleteByUserID(user.UID)
}

// allowSendCode 发码频率限流
func (s *AuthService) allowSendCode(key string) error {
	if !sendCodeMinute.Allow(key) || !sendCodeHour.Allow(key) {
		return model.ErrAttemptTooMany
	}
	return nil
}

// generateUsername 生成随机用户名占位（可后改）
func (s *AuthService) generateUsername() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "user00000000"
	}
	return "user_" + hex.EncodeToString(buf)
}

// asKey 限流 key：直接以邮箱为键
func asKey(email string) string {
	return email
}

// issueTokens 生成双 token 并落库 refresh token
func (s *AuthService) issueTokens(user *model.User) (*model.TokenPair, error) {
	accessToken, err := jwt.GenerateToken(user.UID, user.Username)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.newRefreshToken(user.UID)
	if err != nil {
		return nil, err
	}

	return &model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
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
	if err := s.tokenRepo.Create(record); err != nil {
		return "", err
	}
	return raw, nil
}

// sha256Hex 计算字符串的 SHA-256 十六进制摘要
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
