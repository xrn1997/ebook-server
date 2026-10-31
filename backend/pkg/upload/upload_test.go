package upload

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ebook-server/model"
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
	store := New(t.TempDir(), "")
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

// TestSaveAvatar_RecreatesMissingDir 头像目录被外部清掉后仍能保存（自动重建）。
//
// 钉的是 commit「头像上传目录被清理后自动重建」那个修复：`git clean`、容器重建、
// 手工删目录都会让 uploads/avatar 消失，若写盘前不再兜底 MkdirAll，用户看到的是
// 一次莫名其妙的上传失败——而且只有装过一段时间之后才会撞上。
func TestSaveAvatar_RecreatesMissingDir(t *testing.T) {
	root := t.TempDir()
	store := New(root, "")
	if _, err := os.Stat(store.AvatarDir()); err != nil {
		t.Fatalf("New 应确保目录存在: %v", err)
	}

	if err := os.RemoveAll(store.AvatarDir()); err != nil {
		t.Fatalf("remove avatar dir: %v", err)
	}

	name, err := store.SaveAvatar(newAvatarHeader(t, "me.png", "fake-png-bytes"))
	if err != nil {
		t.Fatalf("目录被清后 SaveAvatar 失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), name)); err != nil {
		t.Errorf("重建后文件没落到预期目录: %v", err)
	}
}

// TestSaveAvatar_RejectsBadExt 非白名单扩展名拒绝。
func TestSaveAvatar_RejectsBadExt(t *testing.T) {
	store := New(t.TempDir(), "")
	if _, err := store.SaveAvatar(newAvatarHeader(t, "evil.txt", "x")); !errors.Is(err, model.ErrInvalidImage) {
		t.Errorf("expected ErrInvalidImage for .txt, got %v", err)
	}
}

// TestSaveAvatar_RejectsOversize 超过 5MB 拒绝。
func TestSaveAvatar_RejectsOversize(t *testing.T) {
	store := New(t.TempDir(), "")
	big := strings.Repeat("a", MaxAvatarBytes+1)
	if _, err := store.SaveAvatar(newAvatarHeader(t, "big.png", big)); !errors.Is(err, model.ErrInvalidImage) {
		t.Errorf("expected ErrInvalidImage for oversize, got %v", err)
	}
}

// TestDeleteAvatar_OwnURL 本服务 URL 的文件被删除。
func TestDeleteAvatar_OwnURL(t *testing.T) {
	store := New(t.TempDir(), "")
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
	store := New(t.TempDir(), "")
	if err := store.DeleteAvatar("https://cdn.example.com/x/y.png"); err != nil {
		t.Errorf("external URL delete should be no-op, got %v", err)
	}
	if err := store.DeleteAvatar(""); err != nil {
		t.Errorf("empty URL should be no-op, got %v", err)
	}
}

// TestDeleteAvatar_RejectsDotSegments "." 与 ".." 不是头像文件名，不能被当成可删除路径。
//
// os.Remove(avatarDir + "/..") 会解析到 uploads 根目录，"/." 则解析到头像目录本身；
// 两者都会让一次「删头像」变成删目录。目录可能恰好为空（或根目录下只剩空头像目录），
// 这时 os.Remove 会成功，下一次上传才由 SaveAvatar 重建——故障表现为偶发、难以归因。
// 拒绝它们也意味着 resolveAvatarName 只返回真正的文件名。
func TestDeleteAvatar_RejectsDotSegments(t *testing.T) {
	store := New(t.TempDir(), "")

	for _, url := range []string{
		"http://h/uploads/avatar/..",
		"http://h/uploads/avatar/.",
	} {
		if err := store.DeleteAvatar(url); err != nil {
			t.Fatalf("DeleteAvatar(%q) = %v, want no-op", url, err)
		}
		if _, err := os.Stat(store.AvatarDir()); err != nil {
			t.Errorf("DeleteAvatar(%q) 删掉了头像目录: %v", url, err)
		}
	}
}

// TestDeleteAvatar_RejectsWindowsNameVariants Windows 在打开/删除文件时会忽略
// 末尾的点与空格：`a.png.` 实际指向 `a.png`。引用计数按字面文件名比较，这类
// 变体会漏掉真实引用；删除入口必须把它们当作非本服务文件。
func TestDeleteAvatar_RejectsWindowsNameVariants(t *testing.T) {
	store := New(t.TempDir(), "")
	name := "normal.png"
	if err := os.WriteFile(filepath.Join(store.AvatarDir(), name), []byte("img"), 0o600); err != nil {
		t.Fatalf("seed avatar: %v", err)
	}

	for _, url := range []string{
		"http://h/uploads/avatar/normal.png.",
		"http://h/uploads/avatar/normal.png ",
	} {
		if err := store.DeleteAvatar(url); err != nil {
			t.Fatalf("DeleteAvatar(%q) = %v, want no-op", url, err)
		}
		if _, err := os.Stat(filepath.Join(store.AvatarDir(), name)); err != nil {
			t.Errorf("DeleteAvatar(%q) 删掉了 %s: %v", url, name, err)
		}
	}
}

// TestDeleteAvatar_TraversalURL 目录穿越形 URL 拒绝删除（防删到 uploads 外文件）。
func TestDeleteAvatar_TraversalURL(t *testing.T) {
	dir := t.TempDir()
	store := New(dir, "")

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

// TestPublicURL 未配置来源时按请求 Host 拼接绝对 URL（ADR-0011 的自托管便利）。
func TestPublicURL(t *testing.T) {
	store := New(t.TempDir(), "")
	got := store.PublicURL("http", "192.168.0.156:9090", "a.png")
	want := "http://192.168.0.156:9090/uploads/avatar/a.png"
	if got != want {
		t.Errorf("PublicURL = %q, want %q", got, want)
	}
}

// TestPublicURL_PrefersConfiguredBase 配置钉死来源后，请求 Host 不再参与拼接。
//
// 自托管没有固定域名，头像 URL 原本直接采请求 Host；伪造一枚 Host 就能把任意外链
// 写进数据库，再经评论列表分发给其他用户。部署者填了 server.public_base_url 之后，
// 这条通道被关掉；尾斜杠的两种写法必须给出同一个结果。
func TestPublicURL_PrefersConfiguredBase(t *testing.T) {
	want := "https://book.example.com/uploads/avatar/a.png"
	for _, base := range []string{"https://book.example.com", "https://book.example.com/"} {
		store := New(t.TempDir(), base)
		if got := store.PublicURL("http", "evil.test:9090", "a.png"); got != want {
			t.Errorf("base = %q 时 PublicURL = %q, want %q", base, got, want)
		}
	}
}

// TestPublicURL_FallsBackToPathWhenHostUntrustworthy Host 形状不可信（带路径、
// 带 scheme、带空白、端口越界）时不回显对方选定的 origin，退回站内路径。
func TestPublicURL_FallsBackToPathWhenHostUntrustworthy(t *testing.T) {
	store := New(t.TempDir(), "")
	for _, host := range []string{"evil.test/x", "http://evil.test", "a b", "evil.test:99999", "user@evil.test", ""} {
		want := "/uploads/avatar/a.png"
		if got := store.PublicURL("http", host, "a.png"); got != want {
			t.Errorf("host = %q 时 PublicURL = %q, want %q", host, got, want)
		}
	}
}
