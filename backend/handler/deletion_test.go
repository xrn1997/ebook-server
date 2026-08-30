package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ebook-server/middleware"

	"github.com/gin-gonic/gin"
)

// setupDeletionRouter 注册注销与导出所需的路由
func setupDeletionRouter(t *testing.T, app *testApp) *gin.Engine {
	t.Helper()

	router := setupRouter()
	authHandler := app.auth
	accountHandler := app.account

	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)

	users := router.Group("/api/users")
	users.Use(middleware.JWTAuth())
	{
		users.GET("/me/data", accountHandler.ExportMyData)
		users.POST("/me/deletion/send-code", accountHandler.SendDeletionCode)
		users.POST("/me/deletion", accountHandler.DeleteAccount)
	}
	return router
}

func TestUserHandler_ExportMyData_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupDeletionRouter(t, app)
	_, token := registerUser(t, router, "mydata-h@example.com")

	req, _ := http.NewRequest("GET", "/api/users/me/data", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	user, ok := data["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'user' object in export, got %v", data)
	}
	if user["email"] != "mydata-h@example.com" {
		t.Errorf("Expected exported email, got %v", user["email"])
	}
	if _, exists := user["password"]; exists {
		t.Error("Export must not contain a password field")
	}
}

func TestUserHandler_ExportMyData_NoAuth(t *testing.T) {
	app := newTestApp(t)

	router := setupDeletionRouter(t, app)

	req, _ := http.NewRequest("GET", "/api/users/me/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_SendDeletionCode_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupDeletionRouter(t, app)
	_, token := registerUser(t, router, "senddel@example.com")

	req, _ := http.NewRequest("POST", "/api/users/me/deletion/send-code", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "00000")
}

func TestUserHandler_DeleteAccount_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupDeletionRouter(t, app)
	_, token := registerUser(t, router, "delete-me@example.com")

	// 直接注入注销验证码，绕过邮件链路（注销码 key 为 del:<email>）
	testCodes.Set("del:delete-me@example.com", testCode)

	body := map[string]string{"code": testCode}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/users/me/deletion", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	user, ok := data["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data payload carrying the export, got %v", data)
	}
	if user["email"] != "delete-me@example.com" {
		t.Errorf("Expected export to carry the pre-deletion email, got %v", user["email"])
	}
}

func TestUserHandler_DeleteAccount_WrongCode(t *testing.T) {
	app := newTestApp(t)

	router := setupDeletionRouter(t, app)
	_, token := registerUser(t, router, "wrong-h@example.com")

	testCodes.Set("del:wrong-h@example.com", testCode)

	body := map[string]string{"code": "000000"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/users/me/deletion", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0132")
}
