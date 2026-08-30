package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ebook-server/config"
	"ebook-server/model"
	pubjwt "ebook-server/pkg/jwt"
	"ebook-server/pkg/testdb"
	"ebook-server/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TestMimeType ES module 必须返回 JS MIME，否则浏览器拒绝执行导致白屏（回归保护）。
func TestMimeType(t *testing.T) {
	cases := []struct{ name, want string }{
		{"assets/index-abc.js", "text/javascript; charset=utf-8"},
		{"assets/app-abc.css", "text/css; charset=utf-8"},
		{"assets/app-abc.json", "application/json; charset=utf-8"},
		{"assets/logo.svg", "image/svg+xml"},
	}
	for _, c := range cases {
		if got := mimeType(c.name, nil); got != c.want {
			t.Errorf("mimeType(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// setup 组装后台测试环境：独立 :memory: 库 + 独立配置 + 后台路由。
func setup(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	config.AppConfig = &config.Config{
		Admin: config.AdminConfig{
			Username:  "admin",
			Password:  "pass123",
			JWTSecret: "admin-test-secret",
			ExpireMin: 30,
		},
	}
	db := testdb.Open(t)
	h := NewHandler(
		repository.NewUserRepository(db),
		repository.NewCommentRepository(db),
		repository.NewLogRepository(db),
	)

	r := gin.New()
	adm := r.Group("/admin")
	{
		adm.POST("/api/login", h.Login)
		api := adm.Group("/api")
		api.Use(AuthMiddleware())
		{
			api.GET("/stats", h.Stats)
			api.GET("/users", h.ListUsers)
			api.GET("/logs", h.ListLogs)
		}
	}
	return r, db
}

// perform 执行请求并返回响应体。
func perform(r *gin.Engine, method, path, body, token string) map[string]interface{} {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp
}

func adminLogin(t *testing.T, r *gin.Engine) string {
	t.Helper()
	resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"pass123"}`, "")
	if resp["code"] != "00000" {
		t.Fatalf("login failed: %v", resp)
	}
	return resp["data"].(map[string]interface{})["token"].(string)
}

func TestAdminLogin_Success(t *testing.T) {
	r, _ := setup(t)
	if tok := adminLogin(t, r); tok == "" {
		t.Error("expected non-empty admin token")
	}
}

func TestAdminLogin_WrongPassword(t *testing.T) {
	r, _ := setup(t)
	resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"wrong"}`, "")
	if resp["code"] != "A0403" {
		t.Errorf("expected A0403, got %v", resp["code"])
	}
}

// TestAdminLogin_RateLimit 失败尝试限流（ADR-0010 验收项）：
// 连续 5 次错误密码后第 6 次起返回 A0241；已锁定窗口内即使密码正确也被拒。
func TestAdminLogin_RateLimit(t *testing.T) {
	r, _ := setup(t)

	for i := 1; i <= 6; i++ {
		resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"wrong"}`, "")
		if i <= 5 {
			if resp["code"] != "A0403" {
				t.Errorf("attempt #%d expected A0403, got %v", i, resp["code"])
			}
		} else if resp["code"] != "A0241" {
			t.Errorf("attempt #%d expected A0241 (rate limited), got %v", i, resp["code"])
		}
	}

	// 已锁定窗口内：正确密码同样被拒（A0241）
	resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"pass123"}`, "")
	if resp["code"] != "A0241" {
		t.Errorf("locked window should reject correct password with A0241, got %v", resp["code"])
	}
}

// TestAdminLogin_SuccessNotCounted 成功登录不消耗限流配额：
// 连续多次成功登录（超过 limit）不应触发限流。
func TestAdminLogin_SuccessNotCounted(t *testing.T) {
	r, _ := setup(t)

	for i := 0; i < loginLimiterLimit+3; i++ {
		resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"pass123"}`, "")
		if resp["code"] != "00000" {
			t.Fatalf("successful login #%d should not be rate-limited, got %v", i+1, resp["code"])
		}
	}
}

func TestAdminAuth_RequiresToken(t *testing.T) {
	r, _ := setup(t)
	resp := perform(r, http.MethodGet, "/admin/api/stats", "", "")
	if resp["code"] != "A0403" {
		t.Errorf("expected A0403 without token, got %v", resp["code"])
	}
}

func TestAdminAuth_RejectsPublicJWT(t *testing.T) {
	r, _ := setup(t)
	// 公开用户 JWT 用 jwt.Secret 签发；管理端用独立 admin.JWTSecret，跨密钥签名应被拒。
	config.AppConfig.JWT.Secret = "public-secret"
	pub, err := pubjwt.GenerateToken(1, "someone")
	if err != nil {
		t.Fatalf("generate public token failed: %v", err)
	}
	resp := perform(r, http.MethodGet, "/admin/api/stats", "", pub)
	if resp["code"] != "A0403" {
		t.Errorf("expected public JWT rejected by admin auth, got %v", resp["code"])
	}
}

func TestAdminStats(t *testing.T) {
	r, db := setup(t)

	// 造一个用户，验证 /api/stats 复用 repository 能取到真实计数。
	users := repository.NewUserRepository(db)
	if err := users.Create(&model.User{
		Email: "admin-stats@example.com", Password: "x", Username: "admin-stats",
	}); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	tok := adminLogin(t, r)
	resp := perform(r, http.MethodGet, "/admin/api/stats", "", tok)
	if resp["code"] != "00000" {
		t.Fatalf("stats failed: %v", resp)
	}
	data := resp["data"].(map[string]interface{})
	if int(data["users"].(float64)) < 1 {
		t.Errorf("expected users>=1, got %v", data["users"])
	}
}

func TestAdminListLogs(t *testing.T) {
	r, db := setup(t)

	// 造一条操作日志，验证 /api/logs 能查到并返回。
	logs := repository.NewLogRepository(db)
	if err := logs.Create(&model.OperationLog{Method: "GET", Path: "/api/comments", ResponseCode: 200}); err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	tok := adminLogin(t, r)
	resp := perform(r, http.MethodGet, "/admin/api/logs", "", tok)
	if resp["code"] != "00000" {
		t.Fatalf("list logs failed: %v", resp)
	}
	data := resp["data"].(map[string]interface{})
	if int(data["total"].(float64)) < 1 {
		t.Errorf("expected at least 1 log, got total=%v", data["total"])
	}
}
