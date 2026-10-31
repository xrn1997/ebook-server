package service

import (
	"errors"
	"testing"

	"ebook-server/model"
	"ebook-server/pkg/code"
)

// setupTestDB / cleanupTestDB / seedCode 及测试环境变量见 test_helper_test.go

// serviceRegister 注册一个用户（验证码经模块真实下发）
func serviceRegister(t *testing.T, s *AuthService, email, password string) *model.User {
	t.Helper()
	user, err := s.Register(testCtx, &model.RegisterRequest{
		Email:    email,
		Code:     seedCode(t, FlowRegister, email),
		Password: password,
	})
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}
	return user
}

func TestAuthService_Register_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-reg@example.com"
	user := serviceRegister(t, authService, email, "password123")

	if user.Email != email {
		t.Errorf("Expected email %q, got %q", email, user.Email)
	}
	if user.UID == 0 {
		t.Error("Expected non-zero uid")
	}
	if user.Username == "" {
		t.Error("Expected auto-generated username")
	}
	if user.Password == "password123" {
		t.Error("Password should be hashed")
	}
}

func TestAuthService_Register_WrongCode(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-reg-wrong@example.com"
	wrong := seedCodeWrong(t, FlowRegister, email)

	_, err := authService.Register(testCtx, &model.RegisterRequest{Email: email, Code: wrong, Password: "password123"})
	if err != model.ErrCodeInvalid {
		t.Errorf("Expected ErrCodeInvalid, got %v", err)
	}
}

// TestAuthService_Register_TooManyAttempts 注册验证码错到上限应回尝试超限（A0241）。
//
// 这条曾经是坏的：校验点写成 `!= ResultOK`，把「尝试超限」和「码不对」「无码」
// 一起折成 ErrCodeInvalid，于是 A0241 在 /api/auth/register 上永远不可达。
func TestAuthService_Register_TooManyAttempts(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-reg-lock@example.com"
	wrong := seedCodeWrong(t, FlowRegister, email)

	var err error
	for i := 0; i < code.MaxAttempts; i++ {
		_, err = authService.Register(testCtx, &model.RegisterRequest{Email: email, Code: wrong, Password: "password123"})
	}
	if err != model.ErrAttemptTooMany {
		t.Errorf("Expected ErrAttemptTooMany after exhausting code attempts, got %v", err)
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-reg-dup@example.com"
	serviceRegister(t, authService, email, "password123")

	// 已注册邮箱只有在验证码校验通过后才会看到 ErrEmailExists（ADR-0006）。
	codeVal := seedCode(t, FlowRegister, email)
	_, err := authService.Register(testCtx, &model.RegisterRequest{Email: email, Code: codeVal, Password: "password123"})
	if err != model.ErrEmailExists {
		t.Errorf("Expected ErrEmailExists, got %v", err)
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-login@example.com"
	serviceRegister(t, authService, email, "password123")

	pair, err := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "password123"})
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if pair.RefreshToken == "" {
		t.Error("RefreshToken should not be empty")
	}
}

func TestAuthService_Login_AccountNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	_, err := authService.Login(testCtx, &model.LoginRequest{Email: "ghost@example.com", Password: "x"})
	if err != model.ErrAccountNotFound {
		t.Errorf("Expected ErrAccountNotFound, got %v", err)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-login-wrong@example.com"
	serviceRegister(t, authService, email, "password123")

	_, err := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "wrong"})
	if err != model.ErrPasswordWrong {
		t.Errorf("Expected ErrPasswordWrong, got %v", err)
	}
}

// TestAuthService_Login_ComparesOnceEvenWithoutAccount 钉住防枚举的「等代价」一侧（ADR-0006）。
//
// 业务码统一只堵住了响应内容；一次请求的耗时差仍然会把「这个邮箱注册过吗」说出来——
// 存在账号要跑 bcrypt（数十毫秒），不存在直接返回。所以不存在的账号也必须比对一次。
func TestAuthService_Login_ComparesOnceEvenWithoutAccount(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-equal-cost@example.com"
	serviceRegister(t, authService, email, "password123")

	var compared []string
	authService.credentials.compare = func(hashed, plain []byte) error {
		compared = append(compared, string(hashed))
		return errors.New("口令不匹配") // 本用例只关心比对发生了几次
	}

	if _, err := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "wrong"}); err != model.ErrPasswordWrong {
		t.Fatalf("存在账号 + 错口令应回 ErrPasswordWrong, got %v", err)
	}
	if len(compared) != 1 {
		t.Fatalf("存在账号时比对 %d 次，期望 1 次", len(compared))
	}

	compared = nil
	if _, err := authService.Login(testCtx, &model.LoginRequest{Email: "nobody@example.com", Password: "wrong"}); err != model.ErrAccountNotFound {
		t.Fatalf("未注册邮箱应回 ErrAccountNotFound, got %v", err)
	}
	if len(compared) != 1 {
		t.Fatalf("账号不存在时也必须走一次等代价比对（防时序枚举），实际 %d 次", len(compared))
	}
	if compared[0] == "" {
		t.Error("占位比对必须落在一枚真实摘要上，否则等于什么都没做")
	}
}

// TestAuthService_Login_LockedAccountStillCompares 锁定分支不得提前返回：
// 省掉那次比对会让「账号存在但被锁」成为第二个可测的时序档。
func TestAuthService_Login_LockedAccountStillCompares(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-locked-cost@example.com"
	serviceRegister(t, authService, email, "password123")

	compared := 0
	authService.credentials.compare = func(hashed, plain []byte) error {
		compared++
		return errors.New("口令不匹配")
	}

	var err error
	for i := 0; i < 6; i++ {
		_, err = authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "wrong"})
	}
	if err != model.ErrAccountLocked {
		t.Fatalf("第 6 次应处于锁定期，回 ErrAccountLocked, got %v", err)
	}
	if compared != 6 {
		t.Fatalf("6 次登录应比对 6 次（含锁定期那一次），实际 %d 次", compared)
	}
}

func TestAuthService_Login_Lockout(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-lock@example.com"
	serviceRegister(t, authService, email, "password123")

	// 连续 5 次错误密码：第 5 次触发锁定
	for i := 0; i < 5; i++ {
		_, err := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "wrong"})
		if i < 4 {
			if err != model.ErrPasswordWrong {
				t.Fatalf("attempt %d expected ErrPasswordWrong, got %v", i+1, err)
			}
		} else {
			if err != model.ErrAttemptTooMany {
				t.Fatalf("expected ErrAttemptTooMany on 5th failure, got %v", err)
			}
		}
	}

	// 锁定期间正确密码也被拒绝
	_, err := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "password123"})
	if err != model.ErrAccountLocked {
		t.Errorf("Expected ErrAccountLocked while locked, got %v", err)
	}
}

func TestAuthService_Refresh_Rotation(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-refresh@example.com"
	serviceRegister(t, authService, email, "password123")

	pair, err := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "password123"})
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	newPair, err := authService.Refresh(testCtx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("Failed to refresh: %v", err)
	}
	if newPair.RefreshToken == pair.RefreshToken {
		t.Error("Refresh token should be rotated")
	}

	_, err = authService.Refresh(testCtx, pair.RefreshToken)
	if err != model.ErrLoginExpired {
		t.Errorf("Expected ErrLoginExpired after rotation, got %v", err)
	}
}

func TestAuthService_ChangePassword_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-changepwd@example.com"
	user := serviceRegister(t, authService, email, "password123")

	if err := authService.ChangePassword(testCtx, user.UID, "password123", "newpass456"); err != nil {
		t.Fatalf("Failed to change password: %v", err)
	}

	_, err := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "newpass456"})
	if err != nil {
		t.Errorf("Expected login with new password to succeed, got %v", err)
	}
}

func TestAuthService_ResetPassword_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-forgot@example.com"
	serviceRegister(t, authService, email, "password123")

	if err := authService.ResetPassword(testCtx, email, seedCode(t, FlowForgot, email), "newpass456"); err != nil {
		t.Fatalf("Failed to reset password: %v", err)
	}

	_, err := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "newpass456"})
	if err != nil {
		t.Errorf("Expected login with new password after reset, got %v", err)
	}
}

func TestAuthService_Logout_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-logout@example.com"
	user := serviceRegister(t, authService, email, "password123")

	// 登录生成 token
	pair, _ := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "password123"})
	if pair.RefreshToken == "" {
		t.Fatal("Expected refresh token after login")
	}

	// 登出
	if err := authService.Logout(testCtx, user.UID); err != nil {
		t.Fatalf("Failed to logout: %v", err)
	}

	// 登出后 refresh token 应失效
	_, err := authService.Refresh(testCtx, pair.RefreshToken)
	if err != model.ErrLoginExpired {
		t.Errorf("Expected ErrLoginExpired after logout, got %v", err)
	}
}

func TestAuthService_ChangePassword_UserNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	err := authService.ChangePassword(testCtx, 999999, "old", "new")
	if err != model.ErrAccountNotFound {
		t.Errorf("应透出 repository 的 model.ErrAccountNotFound，got %v", err)
	}
}

func TestAuthService_ChangePassword_WrongOldPassword(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-changepwd-wrong@example.com"
	user := serviceRegister(t, authService, email, "password123")

	err := authService.ChangePassword(testCtx, user.UID, "wrongold", "newpass")
	if err != model.ErrPasswordWrong {
		t.Errorf("Expected ErrPasswordWrong, got %v", err)
	}
}

func TestAuthService_ResetPassword_WrongCode(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-reset-wrong@example.com"
	serviceRegister(t, authService, email, "password123")

	err := authService.ResetPassword(testCtx, email, seedCodeWrong(t, FlowForgot, email), "newpass456")
	if err != model.ErrCodeInvalid {
		t.Errorf("Expected ErrCodeInvalid, got %v", err)
	}
}

func TestAuthService_ResetPassword_AccountNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "ghost-reset@example.com"

	err := authService.ResetPassword(testCtx, email, seedCode(t, FlowForgot, email), "newpass456")
	if err != model.ErrAccountNotFound {
		t.Errorf("Expected ErrAccountNotFound, got %v", err)
	}
}

func TestAuthService_Login_InvalidTokenRefresh(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	_, err := authService.Refresh(testCtx, "nonexistent-token")
	if err != model.ErrLoginExpired {
		t.Errorf("Expected ErrLoginExpired, got %v", err)
	}
}

// TestAuthService_SendCode_UsesRegisterFlow 注册发码落在注册流程自己的命名空间与配额里。
//
// 「下发」由 VerificationCodes 一处负责，本用例钉住认证模块传的是哪个流程：
// 拿这枚码去校验找回密码流程必须失败（流程互不通用），拿来注册才成功。
// 顺带钉住发码配额按流程隔离：注册发过码不影响找回密码首发。
func TestAuthService_SendCode_UsesRegisterFlow(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	email := "svc-sendcode@example.com"
	if err := testAuth.SendCode(testCtx, email); err != nil {
		t.Fatalf("SendCode failed: %v", err)
	}
	codeVal := testMails.last(email)
	if codeVal == "" {
		t.Fatal("Expected a code mailed to the address")
	}

	if _, err := testAuth.Register(testCtx, &model.RegisterRequest{Email: email, Code: codeVal, Password: "password123"}); err != nil {
		t.Fatalf("Register with the dispatched code failed: %v", err)
	}
	if err := testAuth.ResetPassword(testCtx, email, codeVal, "newpass456"); err != model.ErrCodeInvalid {
		t.Errorf("A register code must not reset a password, got %v", err)
	}
	if err := testAuth.SendForgotCode(testCtx, email); err != nil {
		t.Errorf("Forgot-flow quota should be independent of register, got %v", err)
	}
}
