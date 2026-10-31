package service

import (
	"testing"
)

// TestAuthService_SendForgotCode_UnknownEmail 未注册邮箱请求找回密码应静默成功（ADR-0006）
func TestAuthService_SendForgotCode_UnknownEmail(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	if err := testAuth.SendForgotCode(testCtx, "nobody@example.com"); err != nil {
		t.Fatalf("Expected silent success for unknown email, got %v", err)
	}

	// 不得下发验证码——下发了就等于向能收到该邮件的人泄露"邮箱未注册"
	if n := testMails.countTo("nobody@example.com"); n != 0 {
		t.Errorf("Expected no code mailed for an unregistered email, got %d", n)
	}
}

// TestAuthService_SendForgotCode_KnownEmail 已注册邮箱应正常发码
func TestAuthService_SendForgotCode_KnownEmail(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	serviceRegister(t, testAuth, "known@example.com", "password123")

	if err := testAuth.SendForgotCode(testCtx, "known@example.com"); err != nil {
		t.Fatalf("Expected success for known email, got %v", err)
	}
	if testMails.last("known@example.com") == "" {
		t.Error("Expected a code mailed for a registered email")
	}
}
