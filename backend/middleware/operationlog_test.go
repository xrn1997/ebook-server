package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ebook-server/model"

	"github.com/gin-gonic/gin"
)

// fakeLogWriter 记录最近一次写入的 OperationLog，用于断言审计落库。
type fakeLogWriter struct {
	log *model.OperationLog
	err error
}

func (f *fakeLogWriter) Create(log *model.OperationLog) error {
	f.log = log
	return f.err
}

func TestOperationLog_PersistsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeLogWriter{}
	r := gin.New()
	r.Use(OperationLog(w))
	r.GET("/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	req.Header.Set("User-Agent", "test-agent")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if w.log == nil {
		t.Fatal("expected operation log to be written")
	}
	if w.log.Method != http.MethodGet || w.log.Path != "/hello" {
		t.Errorf("unexpected method/path: %s %s", w.log.Method, w.log.Path)
	}
	if w.log.ResponseCode != http.StatusOK {
		t.Errorf("unexpected response code: %d", w.log.ResponseCode)
	}
	// 关键：RequestBody 绝不能落库（防明文密码），UserAgent 应记录。
	if w.log.RequestBody != "" {
		t.Errorf("RequestBody must be empty, got %q", w.log.RequestBody)
	}
	if w.log.UserAgent != "test-agent" {
		t.Errorf("expected UserAgent recorded, got %q", w.log.UserAgent)
	}
}

// TestOperationLog_WriteErrorDoesNotBlock 写入失败只告警不阻断请求。
func TestOperationLog_WriteErrorDoesNotBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeLogWriter{err: errors.New("boom")}
	r := gin.New()
	r.Use(OperationLog(w))
	r.GET("/x", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("write error must not block request, got %d", rec.Code)
	}
}

// TestOperationLog_FiltersAdminRequests 后台管理自身请求（/admin*）不入审计库。
//
// 日志聚焦客户端（android-ebook 等）请求，后台 UI/API 的访问应被过滤。
func TestOperationLog_FiltersAdminRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeLogWriter{}
	r := gin.New()
	r.Use(OperationLog(w))
	r.GET("/api/comments", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/admin/api/stats", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// /api 在：应记录
	w.log = nil
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/comments", nil))
	if w.log == nil || w.log.Path != "/api/comments" {
		t.Errorf("expected /api/comments recorded, got %+v", w.log)
	}

	// /admin 在：应被过滤，不记录
	w.log = nil
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/admin/api/stats", nil))
	if w.log != nil {
		t.Errorf("admin request must not be recorded, got %+v", w.log)
	}
}
