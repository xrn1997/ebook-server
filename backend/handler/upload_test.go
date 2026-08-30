package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ebook-server/middleware"
	"ebook-server/pkg/upload"
	"ebook-server/repository"
	"ebook-server/service"
)

// uploadMultipartBody 构造 multipart 请求体，返回 (body, contentType)。
func uploadMultipartBody(t *testing.T, field, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fw, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write content: %v", err)
	}
	writer.Close()
	return body, writer.FormDataContentType()
}

// TestUploadAvatar_Success 登录后上传成功，返回可访问的绝对 URL。
func TestUploadAvatar_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	uploadHandler := app.uploads
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/uploads/avatar", middleware.JWTAuth(), uploadHandler.UploadAvatar)

	_, token := registerUser(t, router, "upload@example.com")

	body, contentType := uploadMultipartBody(t, "avatar", "me.png", "fake-png")
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/avatar", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	url := data["url"].(string)
	if !strings.HasPrefix(url, "http://example.com/uploads/avatar/") || !strings.HasSuffix(url, ".png") {
		t.Errorf("unexpected avatar url %q", url)
	}
}

// TestUploadAvatar_NoAuth 未登录上传被拒（登录过期）。
func TestUploadAvatar_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	uploadHandler := app.uploads
	router.POST("/api/uploads/avatar", middleware.JWTAuth(), uploadHandler.UploadAvatar)

	body, contentType := uploadMultipartBody(t, "avatar", "me.png", "fake-png")
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/avatar", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

// TestUploadAvatar_BadFormat 非白名单格式返回 A0400。
func TestUploadAvatar_BadFormat(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	uploadHandler := app.uploads
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/uploads/avatar", middleware.JWTAuth(), uploadHandler.UploadAvatar)

	_, token := registerUser(t, router, "upload_bad@example.com")

	body, contentType := uploadMultipartBody(t, "avatar", "evil.txt", "x")
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/avatar", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

// TestUpdateAvatar_DeletesOldFile 更换头像后旧头像文件被清理（ADR-0011，service 层编排）。
func TestUpdateAvatar_DeletesOldFile(t *testing.T) {
	app := newTestApp(t)

	// 用可控的临时目录构建上传 store + userService（清理逻辑在 service.Update 内）
	uploads := upload.New(t.TempDir())
	userSvc := service.NewUserService(repository.NewUserRepository(app.db), uploads)
	app.user = NewUserHandler(userSvc, app.user.authService)

	router := setupRouter()
	authHandler := app.auth
	uploadHandler := NewUploadHandler(uploads)
	userHandler := app.user
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/uploads/avatar", middleware.JWTAuth(), uploadHandler.UploadAvatar)
	router.PUT("/api/users/me", middleware.JWTAuth(), userHandler.UpdateMe)

	_, token := registerUser(t, router, "avatar_update@example.com")

	// 第一步：上传拿 URL
	body, contentType := uploadMultipartBody(t, "avatar", "old.png", "fake-png")
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/avatar", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	oldURL := decodeData(t, w.Body.Bytes())["url"].(string)

	// 第二步：PUT /api/users/me 提交旧 URL
	jsonBody, _ := json.Marshal(map[string]string{"avatar": oldURL})
	req, _ = http.NewRequest(http.MethodPut, "/api/users/me", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	decodeData(t, w.Body.Bytes())

	// 旧文件此刻应存在（还在被引用）
	oldName := filepath.Base(oldURL)
	oldPath := filepath.Join(uploads.AvatarDir(), oldName)
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old avatar should exist before replacement: %v", err)
	}

	// 再传一个新头像并更新 → 旧文件应被删除
	body, contentType = uploadMultipartBody(t, "avatar", "new.png", "fake-png")
	req = httptest.NewRequest(http.MethodPost, "/api/uploads/avatar", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	newURL := decodeData(t, w.Body.Bytes())["url"].(string)

	jsonBody, _ = json.Marshal(map[string]string{"avatar": newURL})
	req, _ = http.NewRequest(http.MethodPut, "/api/users/me", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	decodeData(t, w.Body.Bytes())

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old avatar file should be deleted after replacement")
	}
	newPath := filepath.Join(uploads.AvatarDir(), filepath.Base(newURL))
	if _, err := os.Stat(newPath); err != nil {
		t.Error("new avatar file should exist")
	}
}
