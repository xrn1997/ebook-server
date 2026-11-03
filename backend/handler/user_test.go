package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserHandler_GetMe_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodGet, "/api/users/me", nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_GetMe_WithAuth(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "auth@example.com")

	w := doJSON(t, router, http.MethodGet, "/api/users/me", nil, token)
	data := decodeData(t, w.Body.Bytes())
	if data["email"] != "auth@example.com" {
		t.Errorf("Expected email 'auth@example.com', got %v", data["email"])
	}
	if data["username"] == nil || data["username"] == "" {
		t.Error("Expected auto-generated username")
	}
}

func TestUserHandler_GetMe_InvalidToken(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodGet, "/api/users/me", nil, "invalid-token")
	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_UpdateMe_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "update@example.com")

	w := doJSON(t, router, http.MethodPut, "/api/users/me", map[string]string{
		"username": "myname",
		"avatar":   "https://example.com/avatar.jpg",
	}, token)
	data := decodeData(t, w.Body.Bytes())
	if data["username"] != "myname" {
		t.Errorf("Expected username 'myname', got %v", data["username"])
	}
	// 登录主标识不可变更（ADR-0004），应保持注册时的值
	if data["email"] != "update@example.com" {
		t.Errorf("Expected email to remain 'update@example.com', got %v", data["email"])
	}
}

// TestUserHandler_UpdateMe_EmailImmutable 改登录主标识应返回 A0113（ADR-0004）
func TestUserHandler_UpdateMe_EmailImmutable(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "immutable-h@example.com")

	w := doJSON(t, router, http.MethodPut, "/api/users/me",
		map[string]string{"email": "another@example.com"}, token)
	assertErrorCode(t, w.Body.Bytes(), "A0113")
}

func TestUserHandler_UpdateMe_InvalidEmail(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "invalid-email@example.com")

	w := doJSON(t, router, http.MethodPut, "/api/users/me",
		map[string]string{"email": "not-an-email"}, token)
	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestUserHandler_ChangePassword_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	email := "changepwd@example.com"
	_, token := registerUser(t, router, email)

	w := doJSON(t, router, http.MethodPut, "/api/users/me/password",
		map[string]string{"old_password": "password123", "new_password": "newpass456"}, token)
	assertErrorCode(t, w.Body.Bytes(), "00000")

	// 新密码可登录
	decodeData(t, doJSON(t, router, http.MethodPost, "/api/auth/login",
		map[string]string{"email": email, "password": "newpass456"}, "").Body.Bytes())
}

func TestUserHandler_ChangePassword_WrongOldPassword(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "wrongold@example.com")

	w := doJSON(t, router, http.MethodPut, "/api/users/me/password",
		map[string]string{"old_password": "wrongpassword", "new_password": "newpass456"}, token)
	assertErrorCode(t, w.Body.Bytes(), "A0210")
}

func TestUserHandler_UpdateMe_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodPut, "/api/users/me",
		map[string]string{"username": "newname"}, "")
	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_UpdateMe_InvalidJSON(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "invalidjson@example.com")

	req, _ := http.NewRequest("PUT", "/api/users/me", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestUserHandler_ChangePassword_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodPut, "/api/users/me/password",
		map[string]string{"old_password": "old", "new_password": "new"}, "")
	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_ChangePassword_InvalidJSON(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "invalidpwd@example.com")

	req, _ := http.NewRequest("PUT", "/api/users/me/password", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}
