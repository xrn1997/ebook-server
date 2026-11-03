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

// TestAuthService_Login_UnknownEmailComparesAgainstPlaceholder 防枚举时序等价的测试抓手：
// 未注册邮箱的登录必须恰花掉一次口令比对，且比对对象是 loginHashPlaceholder 占位摘要
// ——「这个邮箱注册过吗」既问不出在响应码里，也问不出在响应时间里（ADR-0006）。
//
// 计数与入参都从 credentials.compare 这道可注入缝上观察（包内可及），不掐表测时。
func TestAuthService_Login_UnknownEmailComparesAgainstPlaceholder(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	var calls int
	var gotHash string
	testAuth.credentials.compare = func(hashed, plain []byte) error {
		calls++
		gotHash = string(hashed)
		return errors.New("test compare: never matches")
	}

	_, err := testAuth.Login(testCtx, &model.LoginRequest{Email: "nobody-placeholder@example.com", Password: "whatever"})
	if !errors.Is(err, model.ErrAccountNotFound) {
		t.Fatalf("Expected ErrAccountNotFound, got %v", err)
	}
	if calls != 1 {
		t.Errorf("未知邮箱路径的口令比对次数 = %d, want 1", calls)
	}
	if gotHash != loginHashPlaceholder {
		t.Errorf("比对入参 hashed = %q, want loginHashPlaceholder", gotHash)
	}
}

// TestAuthService_Login_UnknownEmailAndPasswordWrongFoldToOneClass 未注册邮箱与错口令
// 的登录失败经凭据模块的折叠规则（IsLoginDenied）归为同一类——handler 因此只判一次
// 就能给两者完全相同的信封（ADR-0006）。
//
// 断言钉在公开面：信封逐字节一致由 handler/enumeration_test.go 用真实路由钉住；
// 「不存在的账号也走一次等代价比对」的时序保证由
// TestAuthService_Login_UnknownEmailComparesAgainstPlaceholder 钉住，本用例不重复该抓手。
func TestAuthService_Login_UnknownEmailAndPasswordWrongFoldToOneClass(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-equal-cost@example.com"
	serviceRegister(t, authService, email, "password123")

	_, wrongErr := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "wrong"})
	if !IsLoginDenied(wrongErr) {
		t.Fatalf("错口令应回可折叠的登录拒绝, got %v", wrongErr)
	}

	_, unknownErr := authService.Login(testCtx, &model.LoginRequest{Email: "nobody@example.com", Password: "wrong"})
	if !IsLoginDenied(unknownErr) {
		t.Fatalf("未注册邮箱应回可折叠的登录拒绝, got %v", unknownErr)
	}
}

// TestAuthService_Login_LockedAccountFoldsIntoSameClass 尝试超限与账号锁定同样折进
// 同一类：这两枚哨兵只可能出现在已注册账号上，若在 handler 漏折，五次错误密码就能
// 判断某个邮箱是否已注册（ADR-0006 的硬规则）。
func TestAuthService_Login_LockedAccountFoldsIntoSameClass(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	email := "svc-locked-cost@example.com"
	serviceRegister(t, authService, email, "password123")

	var err error
	for i := 0; i < 5; i++ {
		_, err = authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "wrong"})
	}
	if err != model.ErrAttemptTooMany {
		t.Fatalf("第 5 次失败应回 ErrAttemptTooMany, got %v", err)
	}
	if !IsLoginDenied(err) {
		t.Fatalf("尝试超限必须可折叠, got %v", err)
	}

	// 锁定期间正确密码也被拒绝，且同样折叠
	if _, err = authService.Login(testCtx, &model.LoginRequest{Email: email, Password: "password123"}); err != model.ErrAccountLocked {
		t.Fatalf("锁定期应回 ErrAccountLocked, got %v", err)
	}
	if !IsLoginDenied(err) {
		t.Fatalf("账号锁定必须可折叠, got %v", err)
	}
}

// TestAuthService_Login_LockedAccountStillCompares 锁定分支不得省掉那次比对：
// 提前返回会让「账号存在但被锁」成为第二个可测的耗时档（ADR-0006 的时序面，
// 生产侧的顺序保证见 backend/service/auth.go 的 Login「比对放在所有分支之前」）。
//
// 折叠只证明两类拒绝回同一个信封，证明不了它们花掉的代价相同；后者要在
// credentials.compare 这道缝上数得出来，与
// TestAuthService_Login_UnknownEmailComparesAgainstPlaceholder 同一个理由。
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
