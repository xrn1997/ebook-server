package service

import (
	"ebook-server/pkg/code"
	"testing"
)

// TestAuthService_SendForgotCode_UnknownEmail 未注册邮箱请求找回密码应静默成功（ADR-0006）
func TestAuthService_SendForgotCode_UnknownEmail(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth

	if err := authService.SendForgotCode("nobody@example.com"); err != nil {
		t.Fatalf("Expected silent success for unknown email, got %v", err)
	}

	// 不得存下验证码——存了就等于向能探测该 key 的人泄露"邮箱未注册"
	if got := testCodes.Verify("forgot:nobody@example.com", ""); got != code.ResultNotFound {
		t.Error("Expected no code stored for an unregistered email")
	}
}

// TestAuthService_SendForgotCode_KnownEmail 已注册邮箱应正常发码
func TestAuthService_SendForgotCode_KnownEmail(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	serviceRegister(t, authService, "known@example.com", "password123")

	if err := authService.SendForgotCode("known@example.com"); err != nil {
		t.Fatalf("Expected success for known email, got %v", err)
	}
	if got := testCodes.Verify("forgot:known@example.com", ""); got == code.ResultNotFound {
		t.Error("Expected a code stored for a registered email")
	}
}
