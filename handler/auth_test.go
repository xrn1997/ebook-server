package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"ebook-server/config"
	"ebook-server/model"
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
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			ExpireHour: 1,
		},
		Database: config.DatabaseConfig{
			Path: "test_handler.db",
		},
	}

	if err := database.Init(); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}

	db := database.GetDB()
	db.AutoMigrate(&model.User{}, &model.Comment{}, &model.OperationLog{})
}

func cleanupHandlerTestDB(t *testing.T) {
	t.Helper()
	sqlDB, _ := database.GetDB().DB()
	sqlDB.Close()
	os.Remove("test_handler.db")
}

func TestAuthHandler_Register_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)

	body := map[string]string{
		"username": "testuser",
		"password": "password123",
		"email":    "test@example.com",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
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

func TestAuthHandler_Register_InvalidBody(t *testing.T) {
	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)

	// 空 body
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestAuthHandler_Register_InvalidJSON(t *testing.T) {
	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)

	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestAuthHandler_Login_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)

	// 先注册
	registerBody := map[string]string{
		"username": "loginuser",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 登录
	loginBody := map[string]string{
		"username": "loginuser",
		"password": "password123",
	}
	jsonBody, _ = json.Marshal(loginBody)
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
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

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be an object")
	}

	if _, ok := data["token"]; !ok {
		t.Error("Expected token in response")
	}
}

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/login", authHandler.Login)

	body := map[string]string{
		"username": "nonexistent",
		"password": "wrongpassword",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestAuthHandler_Login_MissingFields(t *testing.T) {
	router := setupRouter()
	authHandler := NewAuthHandler()
	router.POST("/api/auth/login", authHandler.Login)

	body := map[string]string{
		"username": "testuser",
		// password missing
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}
