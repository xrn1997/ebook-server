package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/jwt"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// setupHandlerTestDB / cleanupHandlerTestDB 已由 test_helper_test.go 的 newTestApp 取代

func TestAuthHandler_Register_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)

	email := "reg-success@example.com"
	codeVal := seedCode(t, service.FlowRegister, email)

	body := map[string]string{"email": email, "code": codeVal, "password": "password123"}
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

// TestAuthHandler_Register_TooLongPassword 多字节密码的字节上限回 A0120 而不是 C0500。
//
// binding 的 max=50 按字符计数：25 个汉字能通过校验，却超过 bcrypt 的 72 字节上限。
// 这条约束由凭据模块判定，响应必须是「密码不合规」这个业务码。
func TestAuthHandler_Register_TooLongPassword(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)

	email := "reg-long-password@example.com"
	body := map[string]string{
		"email":    email,
		"code":     seedCode(t, service.FlowRegister, email),
		"password": strings.Repeat("密", 25), // 75 字节
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0120")
}

func TestAuthHandler_Register_InvalidBody(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)

	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestAuthHandler_Register_InvalidJSON(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)

	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestAuthHandler_Register_WrongCode(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)

	email := "reg-wrongcode@example.com"
	wrong := seedCodeWrong(t, service.FlowRegister, email)

	body := map[string]string{"email": email, "code": wrong, "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0132")
}

func TestAuthHandler_Register_DuplicateEmail(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)

	email := "reg-dup@example.com"
	registerRegister(t, router, email)

	// 再次注册同邮箱
	body := map[string]string{"email": email, "code": seedCode(t, service.FlowRegister, email), "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0112")
}

func TestAuthHandler_Login_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/login", authHandler.Login)

	body := map[string]string{"password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

// failingUserStore 让登录查询账号时返回未登记的错误（模拟数据库故障）。
type failingUserStore struct {
	err error
}

func (s *failingUserStore) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	return nil, s.err
}
func (s *failingUserStore) FindByUID(ctx context.Context, uid uint) (*model.User, error) {
	return nil, s.err
}
func (s *failingUserStore) Update(ctx context.Context, user *model.User) error { return s.err }
func (s *failingUserStore) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return false, s.err
}

// TestAuthHandler_Login_ServerFailureIsNotCredentialError 服务端故障不能被说成凭据错误。
//
// ADR-0006 要抹平的是「凭据对不对」这一维度的差异，不是把基础设施故障也说成
// 密码错误。数据库连不上时回「邮箱或密码错误」会把用户引向反复改密码，而真正
// 的原因是服务不可用；运维也会被错误的方向带走。
func TestAuthHandler_Login_ServerFailureIsNotCredentialError(t *testing.T) {
	access := jwt.New("test-secret-key", 60)
	// 登录在取到账号之前就失败了，credentials 与 token store 都不会被用到。
	credentials := service.NewCredentials(nil, nil)
	authSvc := service.NewAuthService(
		&failingUserStore{err: errors.New("database is down")},
		nil,
		nil,
		access,
		credentials,
	)

	router := setupRouter()
	router.POST("/api/auth/login", NewAuthHandler(authSvc).Login)

	body := map[string]string{"email": "someone@example.com", "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "C0500")
	assertErrorMessage(t, w.Body.Bytes(), "登录失败")
}

func TestAuthHandler_SendCode_RateLimited(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
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
	body := map[string]string{"email": email, "code": seedCode(t, service.FlowRegister, email), "password": "password123"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertErrorCode(t, w.Body.Bytes(), "00000")
}

func TestAuthHandler_Logout_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/logout", middleware.JWTAuth(app.tokens), authHandler.Logout)

	uid, token := registerUser(t, router, "logout@example.com")
	_ = uid

	req, _ := http.NewRequest("POST", "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "00000")
}

func TestAuthHandler_Logout_NoAuth(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/logout", authHandler.Logout)

	req, _ := http.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestAuthHandler_Refresh_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/forgot-password/reset", authHandler.ForgotPasswordReset)

	email := "forgot-reset@example.com"
	registerUser(t, router, email)

	// 经模块下发找回密码验证码
	codeVal := seedCode(t, service.FlowForgot, email)

	body := map[string]string{
		"email":        email,
		"code":         codeVal,
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/forgot-password/reset", authHandler.ForgotPasswordReset)

	email := "forgot-wrong@example.com"
	registerUser(t, router, email)

	body := map[string]string{
		"email":        email,
		"code":         seedCodeWrong(t, service.FlowForgot, email),
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/forgot-password/reset", authHandler.ForgotPasswordReset)

	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/reset", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestAuthHandler_ForgotPasswordReset_AccountNotFound(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/forgot-password/reset", authHandler.ForgotPasswordReset)

	email := "ghost-forgot@example.com"

	body := map[string]string{
		"email":        email,
		"code":         seedCode(t, service.FlowForgot, email),
		"new_password": "newpass456",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/reset", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 未认证端点不得返回 A0201（ADR-0006：该码仅用于已认证上下文）。
	// "验证码通过但账号不存在"属异常态，对外统一按服务器错误处理。
	resp := w.Body.Bytes()
	assertErrorCode(t, resp, "C0500")
	if strings.Contains(string(resp), "账户不存在") {
		t.Errorf("response must not leak account existence: %s", resp)
	}
}

func TestAuthHandler_ForgotPasswordSendCode_InvalidJSON(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/forgot-password/send-code", authHandler.ForgotPasswordSendCode)

	req, _ := http.NewRequest("POST", "/api/auth/forgot-password/send-code", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestAuthHandler_SendCode_InvalidJSON(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	router.POST("/api/auth/send-code", authHandler.SendCode)

	req, _ := http.NewRequest("POST", "/api/auth/send-code", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

// TestAuthHandler_Login_LockoutDoesNotRevealAccountExistence 锁定与尝试超限必须与
// 「账号不存在/密码错误」返回同一信封（ADR-0006）。
//
// A0241/A0242 只可能出现在已注册账号上：若原样返回，攻击者用五次错误密码就能
// 判断某个邮箱是否已注册，枚举保护形同虚设。
func TestAuthHandler_Login_LockoutDoesNotRevealAccountExistence(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	router.POST("/api/auth/register", app.auth.Register)
	router.POST("/api/auth/login", app.auth.Login)

	known := "lockout-known@example.com"
	unknown := "lockout-unknown@example.com"
	registerRegister(t, router, known)

	decode := func(w *httptest.ResponseRecorder) (string, string) {
		t.Helper()
		var resp struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return resp.Code, resp.Error
	}

	for i := 0; i < 6; i++ {
		knownCode, knownError := decode(doJSON(t, router, http.MethodPost, "/api/auth/login",
			map[string]string{"email": known, "password": "wrong-password"}, ""))
		unknownCode, unknownError := decode(doJSON(t, router, http.MethodPost, "/api/auth/login",
			map[string]string{"email": unknown, "password": "wrong-password"}, ""))

		if knownCode != "A0210" || unknownCode != "A0210" {
			t.Fatalf("attempt %d: codes = (%s, %s), want both A0210", i+1, knownCode, unknownCode)
		}
		if knownError != unknownError {
			t.Fatalf("attempt %d: error = (%q, %q), must be identical", i+1, knownError, unknownError)
		}
	}
}

// TestAuthHandler_Register_ExistingEmailNeedsCodeFirst 注册端点的「邮箱已注册」
// 必须在验证码校验通过之后才可见（ADR-0006）。
//
// 否则任意人只凭一个错码就能区分已注册与未注册邮箱；先验码则只有能收到该邮箱
// 验证码的人才会看到 A0112。
func TestAuthHandler_Register_ExistingEmailNeedsCodeFirst(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	router.POST("/api/auth/register", app.auth.Register)

	known := "register-oracle-known@example.com"
	registerRegister(t, router, known)

	for _, email := range []string{known, "register-oracle-unknown@example.com"} {
		w := doJSON(t, router, http.MethodPost, "/api/auth/register", map[string]string{
			"email":    email,
			"code":     seedCodeWrong(t, service.FlowRegister, email),
			"password": "password123",
		}, "")
		assertErrorCode(t, w.Body.Bytes(), "A0132")
	}
}
