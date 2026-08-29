package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ebook-server/config"
	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/jwt"
	"time"

	"github.com/gin-gonic/gin"
)

// testCode 测试用注册验证码
const testCode = "123456"

// TestConfig 测试配置
var TestConfig = &config.Config{
	Server: config.ServerConfig{
		Port: 8080,
		Mode: "test",
	},
	Database: config.DatabaseConfig{
		Path: "test.db",
	},
	JWT: config.JWTConfig{
		Secret:    "test-secret-key",
		ExpireMin: 1,
	},
}

// SetupTestConfig 初始化测试配置
func SetupTestConfig() {
	config.AppConfig = TestConfig
}

// GenerateTestToken 生成测试用 Token
func GenerateTestToken(userID uint, username string) (string, error) {
	return jwt.GenerateToken(userID, username)
}

// CreateTestUser 创建测试用户
func CreateTestUser() *model.User {
	return &model.User{
		UID:      1,
		Email:    "test@example.com",
		Username: "testuser",
		Avatar:   "https://example.com/avatar.jpg",
	}
}

// CreateTestComment 创建测试评论
func CreateTestComment(userID uint) *model.Comment {
	return &model.Comment{
		ID:        1,
		UserID:    userID,
		Content:   "This is a test comment",
		CreatedAt: time.Now(),
	}
}

// CreateTestLog 创建测试日志
func CreateTestLog(userID uint) *model.OperationLog {
	return &model.OperationLog{
		ID:           1,
		UserID:       userID,
		Username:     "testuser",
		Method:       "GET",
		Path:         "/api/test",
		IP:           "127.0.0.1",
		ResponseCode: 200,
		CreatedAt:    time.Now(),
	}
}

// decodeData 解析响应信封并返回 data（成功时 code 为 00000）
func decodeData(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if resp["code"] != "00000" {
		t.Fatalf("Expected success code, got %v (error=%v)", resp["code"], resp["error"])
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be an object")
	}
	return data
}

// registerUser 完整走一遍「注入注册码 → 注册 → 登录」，返回 (uid, accessToken)
func registerUser(t *testing.T, router *gin.Engine, email string) (uint, string) {
	t.Helper()
	return registerUserWith(t, router, email, "password123")
}

// registerUserWith 指定密码的注册+登录助手
func registerUserWith(t *testing.T, router *gin.Engine, email, password string) (uint, string) {
	t.Helper()
	// 直接注入注册码，绕过邮件链路（注册码 key 为 reg:<email>）
	code.Default().Set("reg:"+email, testCode)

	body := map[string]string{
		"email":    email,
		"code":     testCode,
		"password": password,
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// 注册成功返回值不带 token，仅校验业务码
	assertErrorCode(t, w.Body.Bytes(), "00000")

	// 登录取 token 与 uid
	loginBody := map[string]string{"email": email, "password": password}
	jsonBody, _ = json.Marshal(loginBody)
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	data := decodeData(t, w.Body.Bytes())
	user := data["user"].(map[string]interface{})
	return uint(user["uid"].(float64)), data["token"].(string)
}

// authBearer 生成一个可用的 Bearer token
func authBearer(uid uint, username string) string {
	token, _ := jwt.GenerateToken(uid, username)
	return "Bearer " + token
}

// assertErrorCode 断言响应信封中的业务错误码
func assertErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if resp["code"] != want {
		t.Errorf("Expected code %s, got %v (error=%v)", want, resp["code"], resp["error"])
	}
}
