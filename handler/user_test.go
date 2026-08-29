package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ebook-server/middleware"
	"ebook-server/pkg/jwt"
)

func TestUserHandler_GetMe_NoAuth(t *testing.T) {
	router := setupRouter()
	userHandler := NewUserHandler()
	router.GET("/api/users/me", userHandler.GetMe)

	req, _ := http.NewRequest("GET", "/api/users/me", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestUserHandler_GetMe_WithAuth(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	// 先注册用户
	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.GET("/api/users/me", middleware.JWTAuth(), userHandler.GetMe)

	// 注册
	registerBody := map[string]string{
		"username": "authuser",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 获取 Token
	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))

	token, _ := jwt.GenerateToken(userID, "authuser")

	// 获取用户信息
	req, _ = http.NewRequest("GET", "/api/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["code"] != float64(200) {
		t.Errorf("Expected code 200, got %v", response["code"])
	}
}

func TestUserHandler_GetMe_InvalidToken(t *testing.T) {
	router := setupRouter()
	userHandler := NewUserHandler()
	router.GET("/api/users/me", middleware.JWTAuth(), userHandler.GetMe)

	req, _ := http.NewRequest("GET", "/api/users/me", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestUserHandler_UpdateMe_NoAuth(t *testing.T) {
	router := setupRouter()
	userHandler := NewUserHandler()
	router.PUT("/api/users/me", userHandler.UpdateMe)

	body := map[string]string{
		"email": "new@example.com",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", "/api/users/me", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestUserHandler_UpdateMe_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	// 先注册用户
	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.PUT("/api/users/me", middleware.JWTAuth(), userHandler.UpdateMe)

	// 注册
	registerBody := map[string]string{
		"username": "updateuser",
		"password": "password123",
		"email":    "old@example.com",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 获取 Token
	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))

	token, _ := jwt.GenerateToken(userID, "updateuser")

	// 更新用户
	updateBody := map[string]string{
		"email":  "new@example.com",
		"avatar": "https://example.com/avatar.jpg",
	}
	jsonBody, _ = json.Marshal(updateBody)
	req, _ = http.NewRequest("PUT", "/api/users/me", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["code"] != float64(200) {
		t.Errorf("Expected code 200, got %v", response["code"])
	}

	// 验证更新
	userData := response["data"].(map[string]interface{})
	if userData["email"] != "new@example.com" {
		t.Errorf("Expected email 'new@example.com', got '%v'", userData["email"])
	}

	if userData["avatar"] != "https://example.com/avatar.jpg" {
		t.Errorf("Expected avatar 'https://example.com/avatar.jpg', got '%v'", userData["avatar"])
	}
}

func TestUserHandler_UpdateMe_InvalidEmail(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	// 先注册用户
	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.PUT("/api/users/me", middleware.JWTAuth(), userHandler.UpdateMe)

	// 注册
	registerBody := map[string]string{
		"username": "invalidemailuser",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 获取 Token
	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))

	token, _ := jwt.GenerateToken(userID, "invalidemailuser")

	// 使用无效邮箱更新
	updateBody := map[string]string{
		"email": "not-an-email",
	}
	jsonBody, _ = json.Marshal(updateBody)
	req, _ = http.NewRequest("PUT", "/api/users/me", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestUserHandler_UpdateMe_PartialUpdate(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	// 先注册用户
	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.GET("/api/users/me", middleware.JWTAuth(), userHandler.GetMe)
	router.PUT("/api/users/me", middleware.JWTAuth(), userHandler.UpdateMe)

	// 注册
	registerBody := map[string]string{
		"username": "partialupdateuser",
		"password": "password123",
		"email":    "original@example.com",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 获取 Token
	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))

	token, _ := jwt.GenerateToken(userID, "partialupdateuser")

	// 只更新头像
	updateBody := map[string]string{
		"avatar": "https://example.com/new-avatar.jpg",
	}
	jsonBody, _ = json.Marshal(updateBody)
	req, _ = http.NewRequest("PUT", "/api/users/me", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// 获取用户信息验证
	req, _ = http.NewRequest("GET", "/api/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	userData := response["data"].(map[string]interface{})

	// 邮箱应该保持不变
	if userData["email"] != "original@example.com" {
		t.Errorf("Expected email to remain 'original@example.com', got '%v'", userData["email"])
	}

	// 头像应该被更新
	if userData["avatar"] != "https://example.com/new-avatar.jpg" {
		t.Errorf("Expected avatar 'https://example.com/new-avatar.jpg', got '%v'", userData["avatar"])
	}
}
