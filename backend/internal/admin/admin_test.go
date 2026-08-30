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
	h := NewHandler(repository.NewUserRepository(db), repository.NewCommentRepository(db))

	r := gin.New()
	adm := r.Group("/admin")
	{
		adm.POST("/api/login", h.Login)
		api := adm.Group("/api")
		api.Use(AuthMiddleware())
		{
			api.GET("/stats", h.Stats)
			api.GET("/users", h.ListUsers)
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
