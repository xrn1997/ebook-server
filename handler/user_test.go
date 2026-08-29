package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ebook-server/middleware"
)

func TestUserHandler_GetMe_NoAuth(t *testing.T) {
	router := setupRouter()
	userHandler := NewUserHandler()
	router.GET("/api/users/me", userHandler.GetMe)

	req, _ := http.NewRequest("GET", "/api/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_GetMe_WithAuth(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.GET("/api/users/me", middleware.JWTAuth(), userHandler.GetMe)

	_, token := registerUser(t, router, "auth@example.com")

	req, _ := http.NewRequest("GET", "/api/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	if data["email"] != "auth@example.com" {
		t.Errorf("Expected email 'auth@example.com', got %v", data["email"])
	}
	if data["username"] == nil || data["username"] == "" {
		t.Error("Expected auto-generated username")
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

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_UpdateMe_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.PUT("/api/users/me", middleware.JWTAuth(), userHandler.UpdateMe)

	_, token := registerUser(t, router, "update@example.com")

	body := map[string]string{
		"username": "myname",
		"email":    "new@example.com",
		"avatar":   "https://example.com/avatar.jpg",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/api/users/me", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	if data["username"] != "myname" {
		t.Errorf("Expected username 'myname', got %v", data["username"])
	}
	if data["email"] != "new@example.com" {
		t.Errorf("Expected email 'new@example.com', got %v", data["email"])
	}
}

func TestUserHandler_UpdateMe_InvalidEmail(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.PUT("/api/users/me", middleware.JWTAuth(), userHandler.UpdateMe)

	_, token := registerUser(t, router, "invalid-email@example.com")

	body := map[string]string{"email": "not-an-email"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/api/users/me", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestUserHandler_ChangePassword_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.PUT("/api/users/me/password", middleware.JWTAuth(), userHandler.ChangePassword)

	email := "changepwd@example.com"
	_, token := registerUser(t, router, email)

	body := map[string]string{"old_password": "password123", "new_password": "newpass456"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/api/users/me/password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assertErrorCode(t, w.Body.Bytes(), "00000")

	// 新密码可登录，旧密码不可
	loginBody := map[string]string{"email": email, "password": "newpass456"}
	jsonBody, _ = json.Marshal(loginBody)
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	decodeData(t, w.Body.Bytes())
}

func TestUserHandler_ChangePassword_WrongOldPassword(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.PUT("/api/users/me/password", middleware.JWTAuth(), userHandler.ChangePassword)

	_, token := registerUser(t, router, "wrongold@example.com")

	body := map[string]string{"old_password": "wrongpassword", "new_password": "newpass456"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/api/users/me/password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0210")
}

func TestUserHandler_UpdateMe_NoAuth(t *testing.T) {
	router := setupRouter()
	userHandler := NewUserHandler()
	router.PUT("/api/users/me", middleware.JWTAuth(), userHandler.UpdateMe)

	body := map[string]string{"username": "newname"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/api/users/me", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_UpdateMe_InvalidJSON(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.PUT("/api/users/me", middleware.JWTAuth(), userHandler.UpdateMe)

	_, token := registerUser(t, router, "invalidjson@example.com")

	req, _ := http.NewRequest("PUT", "/api/users/me", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestUserHandler_ChangePassword_NoAuth(t *testing.T) {
	router := setupRouter()
	userHandler := NewUserHandler()
	router.PUT("/api/users/me/password", middleware.JWTAuth(), userHandler.ChangePassword)

	body := map[string]string{"old_password": "old", "new_password": "new"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "/api/users/me/password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_ChangePassword_InvalidJSON(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	userHandler := NewUserHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.PUT("/api/users/me/password", middleware.JWTAuth(), userHandler.ChangePassword)

	_, token := registerUser(t, router, "invalidpwd@example.com")

	req, _ := http.NewRequest("PUT", "/api/users/me/password", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}
