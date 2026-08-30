package upload

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newAvatarHeader 构造一个 multipart.FileHeader（模拟客户端上传的 avatar 文件）。
func newAvatarHeader(t *testing.T, filename, content string) *multipart.FileHeader {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fw, err := writer.CreateFormFile("avatar", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write form content: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}
	_, header, err := req.FormFile("avatar")
	if err != nil {
		t.Fatalf("get form file: %v", err)
	}
	return header
}

// TestSaveAvatar_Success 合法头像保存成功，文件落盘、命名带随机段。
func TestSaveAvatar_Success(t *testing.T) {
	store := New(t.TempDir())
	name, err := store.SaveAvatar(newAvatarHeader(t, "me.png", "fake-png-bytes"))
	if err != nil {
		t.Fatalf("SaveAvatar failed: %v", err)
	}
	if !strings.HasSuffix(name, ".png") || len(name) < 20 {
		t.Errorf("unexpected file name %q", name)
	}
	full := filepath.Join(store.avatarDir, name)
	if _, err := os.Stat(full); err != nil {
		t.Errorf("saved file not found: %v", err)
	}
}

// TestSaveAvatar_RejectsBadExt 非白名单扩展名拒绝。
func TestSaveAvatar_RejectsBadExt(t *testing.T) {
	store := New(t.TempDir())
	if _, err := store.SaveAvatar(newAvatarHeader(t, "evil.txt", "x")); err != ErrInvalidImage {
		t.Errorf("expected ErrInvalidImage for .txt, got %v", err)
	}
}

// TestSaveAvatar_RejectsOversize 超过 5MB 拒绝。
func TestSaveAvatar_RejectsOversize(t *testing.T) {
	store := New(t.TempDir())
	big := strings.Repeat("a", MaxAvatarBytes+1)
	if _, err := store.SaveAvatar(newAvatarHeader(t, "big.png", big)); err != ErrInvalidImage {
		t.Errorf("expected ErrInvalidImage for oversize, got %v", err)
	}
}

// TestDeleteAvatar_OwnURL 本服务 URL 的文件被删除。
func TestDeleteAvatar_OwnURL(t *testing.T) {
	store := New(t.TempDir())
	name, err := store.SaveAvatar(newAvatarHeader(t, "me.png", "data"))
	if err != nil {
		t.Fatalf("SaveAvatar failed: %v", err)
	}
	url := store.PublicURL("http", "192.168.0.156:9090", name)
	if err := store.DeleteAvatar(url); err != nil {
		t.Fatalf("DeleteAvatar failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.avatarDir, name)); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

// TestDeleteAvatar_ExternalURL 外部 URL 是 no-op，不删除任何文件。
func TestDeleteAvatar_ExternalURL(t *testing.T) {
	store := New(t.TempDir())
	if err := store.DeleteAvatar("https://cdn.example.com/x/y.png"); err != nil {
		t.Errorf("external URL delete should be no-op, got %v", err)
	}
	if err := store.DeleteAvatar(""); err != nil {
		t.Errorf("empty URL should be no-op, got %v", err)
	}
}

// TestDeleteAvatar_TraversalURL 目录穿越形 URL 拒绝删除（防删到 uploads 外文件）。
func TestDeleteAvatar_TraversalURL(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)

	// 在 uploads 根下造一个"非头像"文件，模拟穿越目标
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("x"), 0o644); err != nil {
		t.Fatalf("create victim: %v", err)
	}

	url := "http://h/uploads/avatar/../victim.txt"
	if err := store.DeleteAvatar(url); err != nil {
		t.Fatalf("DeleteAvatar failed: %v", err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Error("victim file must not be deleted by traversal URL")
	}
}

// TestPublicURL 拼接请求 Host 的绝对 URL。
func TestPublicURL(t *testing.T) {
	store := New(t.TempDir())
	got := store.PublicURL("http", "192.168.0.156:9090", "a.png")
	want := "http://192.168.0.156:9090/uploads/avatar/a.png"
	if got != want {
		t.Errorf("PublicURL = %q, want %q", got, want)
	}
}
