package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// loginAs 以给定邮箱密码调登录接口，返回原始响应体
func loginAs(t *testing.T, router *gin.Engine, email, password string) []byte {
	t.Helper()
	body := map[string]string{"email": email, "password": password}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Body.Bytes()
}

// TestAuthHandler_Login_UnknownEmail 未注册邮箱登录应与密码错误同码，不暴露账号是否存在（ADR-0006）
func TestAuthHandler_Login_UnknownEmail(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/login", authHandler.Login)

	resp := loginAs(t, router, "nobody@example.com", "password123")

	// 必须是 A0210（密码错误），不得是 A0201（账户不存在）
	assertErrorCode(t, resp, "A0210")
	assertErrorMessage(t, resp, loginFailedMessage)
}

// TestAuthHandler_Login_UnknownEmailIdenticalToWrongPassword 未注册邮箱与密码错误的响应必须完全一致（ADR-0006）
//
// 只统一业务码是不够的：error 文案若有差异，攻击者仍可据此枚举已注册邮箱。
// 因此直接比对两次登录的完整响应体。
func TestAuthHandler_Login_UnknownEmailIdenticalToWrongPassword(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)

	// 注册一个真实账号，之后用错误密码登录
	registerUser(t, router, "known@example.com")

	unknownEmail := loginAs(t, router, "nobody@example.com", "password123")
	wrongPassword := loginAs(t, router, "known@example.com", "wrong-password")

	if string(unknownEmail) != string(wrongPassword) {
		t.Errorf("Responses must be identical to prevent account enumeration.\n"+
			"  unknown email: %s\n  wrong password: %s", unknownEmail, wrongPassword)
	}
}

// TestAuthHandler_ForgotPasswordSendCode_UnknownEmail 未注册邮箱请求发码应静默成功（ADR-0006）
func TestAuthHandler_ForgotPasswordSendCode_UnknownEmail(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/forgot-password/send-code", authHandler.ForgotPasswordSendCode)

	body := map[string]string{"email": "nobody@example.com"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/send-code", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "00000")
}
