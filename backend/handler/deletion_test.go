package handler_test

import (
	"net/http"
	"testing"

	"ebook-server/service"
)

func TestUserHandler_ExportMyData_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "mydata-h@example.com")

	w := doJSON(t, router, http.MethodGet, "/api/users/me/data", nil, token)
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
	router := app.engine

	w := doJSON(t, router, http.MethodGet, "/api/users/me/data", nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestUserHandler_SendDeletionCode_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "senddel@example.com")

	w := doJSON(t, router, http.MethodPost, "/api/users/me/deletion/send-code", map[string]string{}, token)
	assertErrorCode(t, w.Body.Bytes(), "00000")
}

func TestUserHandler_DeleteAccount_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "delete-me@example.com")

	// 经模块真实下发注销码（存储键格式是模块的私有细节）
	body := map[string]string{"code": seedCode(t, service.FlowDeletion, "delete-me@example.com")}
	w := doJSON(t, router, http.MethodPost, "/api/users/me/deletion", body, token)
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
	router := app.engine

	_, token := registerUser(t, router, "wrong-h@example.com")

	body := map[string]string{"code": seedCodeWrong(t, service.FlowDeletion, "wrong-h@example.com")}
	w := doJSON(t, router, http.MethodPost, "/api/users/me/deletion", body, token)
	assertErrorCode(t, w.Body.Bytes(), "A0132")
}
