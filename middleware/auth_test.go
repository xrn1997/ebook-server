package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ebook-server/config"
	"ebook-server/pkg/jwt"

	"github.com/gin-gonic/gin"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func setupMiddlewareTestConfig() {
	config.AppConfig = &config.Config{
		JWT: config.JWTConfig{
			Secret:    "test-secret",
			ExpireMin: 1,
		},
	}
}

func TestJWTAuth_ValidToken(t *testing.T) {
	setupMiddlewareTestConfig()

	router := setupTestRouter()
	router.GET("/protected", JWTAuth(), func(c *gin.Context) {
		userID, _ := GetCurrentUserID(c)
		username, _ := GetCurrentUsername(c)
		c.JSON(http.StatusOK, gin.H{"user_id": userID, "username": username})
	})

	token, _ := jwt.GenerateToken(42, "testuser")
	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["user_id"].(float64) != 42 {
		t.Errorf("Expected user_id 42, got %v", resp["user_id"])
	}
	if resp["username"] != "testuser" {
		t.Errorf("Expected username 'testuser', got %v", resp["username"])
	}
}

func TestJWTAuth_MissingHeader(t *testing.T) {
	setupMiddlewareTestConfig()

	router := setupTestRouter()
	router.GET("/protected", JWTAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "A0230" {
		t.Errorf("Expected code A0230, got %v", resp["code"])
	}
}

func TestJWTAuth_InvalidFormat(t *testing.T) {
	setupMiddlewareTestConfig()

	router := setupTestRouter()
	router.GET("/protected", JWTAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "NotBearer token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "A0230" {
		t.Errorf("Expected code A0230, got %v", resp["code"])
	}
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	setupMiddlewareTestConfig()

	router := setupTestRouter()
	router.GET("/protected", JWTAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-string")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "A0230" {
		t.Errorf("Expected code A0230, got %v", resp["code"])
	}
}

func TestJWTAuth_ExpiredToken(t *testing.T) {
	// 使用过期时间极短的配置
	config.AppConfig = &config.Config{
		JWT: config.JWTConfig{
			Secret:    "test-secret",
			ExpireMin: 0, // 0 分钟 → 立即过期
		},
	}

	router := setupTestRouter()
	router.GET("/protected", JWTAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	token, _ := jwt.GenerateToken(1, "testuser")
	// 等待一小段时间确保 token 过期
	time.Sleep(10 * time.Millisecond)

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "A0230" {
		t.Errorf("Expected code A0230 for expired token, got %v", resp["code"])
	}
}

func TestJWTAuth_WrongSecret(t *testing.T) {
	// 用一个密钥生成 token
	config.AppConfig = &config.Config{
		JWT: config.JWTConfig{Secret: "secret-a", ExpireMin: 60},
	}
	token, _ := jwt.GenerateToken(1, "testuser")

	// 用另一个密钥验证
	config.AppConfig.JWT.Secret = "secret-b"

	router := setupTestRouter()
	router.GET("/protected", JWTAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "A0230" {
		t.Errorf("Expected code A0230, got %v", resp["code"])
	}
}

func TestGetCurrentUserID_NotSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, exists := GetCurrentUserID(c)
	if exists {
		t.Error("Expected GetCurrentUserID to return false when not set")
	}
}

func TestGetCurrentUsername_NotSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, exists := GetCurrentUsername(c)
	if exists {
		t.Error("Expected GetCurrentUsername to return false when not set")
	}
}

func TestGetCurrentUserID_Set(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("user_id", uint(42))

	id, exists := GetCurrentUserID(c)
	if !exists {
		t.Error("Expected GetCurrentUserID to return true")
	}
	if id != 42 {
		t.Errorf("Expected user_id 42, got %d", id)
	}
}

func TestGetCurrentUsername_Set(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("username", "testuser")

	name, exists := GetCurrentUsername(c)
	if !exists {
		t.Error("Expected GetCurrentUsername to return true")
	}
	if name != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", name)
	}
}
