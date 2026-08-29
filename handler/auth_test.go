package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ebook-server/config"
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/database"

	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func setupHandlerTestDB(t *testing.T) {
	t.Helper()

	config.AppConfig = &config.Config{
		Server: config.ServerConfig{Mode: "test"},
		JWT: config.JWTConfig{
			Secret:    "test-secret",
			ExpireMin: 1,
		},
		Database: config.DatabaseConfig{
			Path: ":memory:",
		},
	}

	if err := database.Init(); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}

	db := database.GetDB()
	db.AutoMigrate(&model.User{}, &model.Comment{}, &model.OperationLog{}, &model.RefreshToken{})
}

func cleanupHandlerTestDB(t *testing.T) {
	t.Helper()
	sqlDB, _ := database.GetDB().DB()
	sqlDB.Close()
	database.DB = nil
}

func TestAuthHandler_Register_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)

	email := "reg-success@example.com"
	code.Default().Set("reg:"+email, testCode)

	body := map[string]string{"email": email, "code": testCode, "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "00000")

	// 注册（激活）后应立即可登录
	_, token := registerUser(t, router, "reg-success2@example.com") // 确保 login 路由可用
	if token == "" {
		t.Error("login token should not be empty")
	}
}

func TestAuthHandler_Register_InvalidBody(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)

	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestAuthHandler_Register_InvalidJSON(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)

	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestAuthHandler_Register_WrongCode(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)

	email := "reg-wrongcode@example.com"
	code.Default().Set("reg:"+email, "000000")

	body := map[string]string{"email": email, "code": "111111", "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0132")
}

func TestAuthHandler_Register_DuplicateEmail(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)

	email := "reg-dup@example.com"
	registerRegister(t, router, email)

	// 再次注册同邮箱
	code.Default().Set("reg:"+email, testCode)
	body := map[string]string{"email": email, "code": testCode, "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0112")
}

func TestAuthHandler_Login_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)

	email := "login-success@example.com"
	registerRegister(t, router, email)

	body := map[string]string{"email": email, "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	if _, ok := data["token"]; !ok {
		t.Error("Expected token in login response")
	}
	if _, ok := data["refresh_token"]; !ok {
		t.Error("Expected refresh_token in login response")
	}
}

func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)

	email := "login-wrong@example.com"
	registerRegister(t, router, email)

	body := map[string]string{"email": email, "password": "wrongpass"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0210")
}

func TestAuthHandler_Login_Nonexistent(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/login", authHandler.Login)

	body := map[string]string{"email": "ghost@example.com", "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0210")
}

func TestAuthHandler_Login_MissingFields(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/login", authHandler.Login)

	body := map[string]string{"password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestAuthHandler_SendCode_RateLimited(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/send-code", authHandler.SendCode)

	email := "rate@example.com"
	for i := 0; i < 6; i++ {
		body := map[string]string{"email": email}
		jsonBody, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/api/auth/send-code", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		// 并发限流为 1/分钟，第二次起即被拒
		if i == 0 {
			assertErrorCode(t, w.Body.Bytes(), "00000")
		} else {
			assertErrorCode(t, w.Body.Bytes(), "A0241")
		}
	}
}

func TestAuthHandler_Refresh_InvalidToken(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/refresh", authHandler.Refresh)

	body := map[string]string{"refresh_token": "bogus-token"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/refresh", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

// registerRegister 仅执行注册（不登录），用于注册场景测试
func registerRegister(t *testing.T, router *gin.Engine, email string) {
	t.Helper()
	code.Default().Set("reg:"+email, testCode)
	body := map[string]string{"email": email, "code": testCode, "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertErrorCode(t, w.Body.Bytes(), "00000")
}

func TestAuthHandler_Logout_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/logout", middleware.JWTAuth(), authHandler.Logout)

	uid, token := registerUser(t, router, "logout@example.com")
	_ = uid

	req, _ := http.NewRequest("POST", "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "00000")
}

func TestAuthHandler_Logout_NoAuth(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/logout", authHandler.Logout)

	req, _ := http.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestAuthHandler_Refresh_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/refresh", authHandler.Refresh)

	email := "refresh-success@example.com"
	registerUser(t, router, email)

	// 登录取 refresh token
	loginBody := map[string]string{"email": email, "password": "password123"}
	jsonBody, _ := json.Marshal(loginBody)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	loginData := decodeData(t, w.Body.Bytes())
	refreshToken := loginData["refresh_token"].(string)

	// 刷新
	refreshBody := map[string]string{"refresh_token": refreshToken}
	jsonBody, _ = json.Marshal(refreshBody)
	req, _ = http.NewRequest("POST", "/api/auth/refresh", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	if _, ok := data["token"]; !ok {
		t.Error("Expected token in refresh response")
	}
	if _, ok := data["refresh_token"]; !ok {
		t.Error("Expected refresh_token in refresh response")
	}
}

func TestAuthHandler_ForgotPasswordSendCode_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/forgot-password/send-code", authHandler.ForgotPasswordSendCode)

	email := "forgot-send@example.com"
	registerUser(t, router, email)

	body := map[string]string{"email": email}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/send-code", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "00000")
}

func TestAuthHandler_ForgotPasswordSendCode_RateLimited(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/forgot-password/send-code", authHandler.ForgotPasswordSendCode)

	email := "forgot-rate@example.com"
	registerUser(t, router, email)

	// 第一次成功
	body := map[string]string{"email": email}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/send-code", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertErrorCode(t, w.Body.Bytes(), "00000")

	// 第二次限流
	jsonBody, _ = json.Marshal(body)
	req, _ = http.NewRequest("POST", "/api/auth/forgot-password/send-code", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertErrorCode(t, w.Body.Bytes(), "A0241")
}

func TestAuthHandler_ForgotPasswordReset_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/forgot-password/reset", authHandler.ForgotPasswordReset)

	email := "forgot-reset@example.com"
	registerUser(t, router, email)

	// 注入找回密码验证码
	code.Default().Set("forgot:"+email, testCode)

	body := map[string]string{
		"email":        email,
		"code":         testCode,
		"new_password": "newpass456",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/reset", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "00000")

	// 验证新密码可以登录
	loginBody := map[string]string{"email": email, "password": "newpass456"}
	jsonBody, _ = json.Marshal(loginBody)
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	decodeData(t, w.Body.Bytes())
}

func TestAuthHandler_ForgotPasswordReset_WrongCode(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/forgot-password/reset", authHandler.ForgotPasswordReset)

	email := "forgot-wrong@example.com"
	registerUser(t, router, email)

	code.Default().Set("forgot:"+email, "000000")

	body := map[string]string{
		"email":        email,
		"code":         "111111",
		"new_password": "newpass456",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/reset", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0132")
}

func TestAuthHandler_ForgotPasswordReset_InvalidJSON(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/forgot-password/reset", authHandler.ForgotPasswordReset)

	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/reset", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestAuthHandler_ForgotPasswordReset_AccountNotFound(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/forgot-password/reset", authHandler.ForgotPasswordReset)

	email := "ghost-forgot@example.com"
	code.Default().Set("forgot:"+email, testCode)

	body := map[string]string{
		"email":        email,
		"code":         testCode,
		"new_password": "newpass456",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/reset", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0201")
}

func TestAuthHandler_ForgotPasswordSendCode_InvalidJSON(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/forgot-password/send-code", authHandler.ForgotPasswordSendCode)

	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/send-code", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestAuthHandler_SendCode_InvalidJSON(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/send-code", authHandler.SendCode)

	req, _ := http.NewRequest("POST", "/api/auth/send-code", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}
