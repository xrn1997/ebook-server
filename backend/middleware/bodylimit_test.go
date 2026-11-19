package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newBodyLimitEngine 造一个只挂 BodyLimit 的引擎：默认档 + 一个路径覆盖端点，
// 另加一条显式读取 body 的端点，用来验证 chunked（Content-Length 未知）路径。
func newBodyLimitEngine(defaultMax int64, overrides map[string]int64) *gin.Engine {
	r := gin.New()
	r.Use(BodyLimit(defaultMax, overrides))
	r.POST("/api/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.POST("/api/uploads/avatar", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.POST("/api/read", func(c *gin.Context) {
		if _, err := io.Copy(io.Discard, c.Request.Body); err != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large")
			return
		}
		c.String(http.StatusOK, "ok")
	})
	return r
}

var testOverrides = map[string]int64{"/api/uploads/avatar": 64}

func TestBodyLimit_RejectsOversizedJSON(t *testing.T) {
	r := newBodyLimitEngine(8, testOverrides)
	req := httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader("0123456789"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200（业务错误也走统一信封）", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "A0400") || !strings.Contains(body, "请求体过大") {
		t.Fatalf("body = %s, want A0400 请求体过大", body)
	}
}

func TestBodyLimit_AllowsOverridePathAboveDefaultLimit(t *testing.T) {
	r := newBodyLimitEngine(8, testOverrides)
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/avatar", strings.NewReader("0123456789"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("覆盖路径被默认上限误伤: status=%d body=%s", w.Code, w.Body.String())
	}
}

// TestBodyLimit_BoundaryIsInclusive 恰好等于上限必须放行：拒了合法请求，
// 客户端没有任何办法把自己缩到「小一个字节」以外，256 KiB 的评论报文会永远失败。
func TestBodyLimit_BoundaryIsInclusive(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		bodyLen int
		accept  bool
	}{
		{"默认档恰好等于上限", "/api/x", 8, true},
		{"默认档超一字节", "/api/x", 9, false},
		{"覆盖档恰好等于上限", "/api/uploads/avatar", 64, true},
		{"覆盖档超一字节", "/api/uploads/avatar", 65, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newBodyLimitEngine(8, testOverrides)
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(strings.Repeat("a", tc.bodyLen)))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			overLimit := strings.Contains(w.Body.String(), "请求体过大")
			if overLimit == tc.accept {
				t.Fatalf("len=%d accept=%v, got overLimit=%v body=%s", tc.bodyLen, tc.accept, overLimit, w.Body.String())
			}
		})
	}
}

// TestBodyLimit_NilOverridesIsPlainDefault 后台表面传 nil 覆盖表：所有路径统一默认档。
func TestBodyLimit_NilOverridesIsPlainDefault(t *testing.T) {
	r := newBodyLimitEngine(8, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/uploads/avatar", strings.NewReader("0123456789"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "请求体过大") {
		t.Fatalf("nil 覆盖表下超限未被拒: body=%s", w.Body.String())
	}
}

func TestBodyLimit_ChunkedBodyIsCapped(t *testing.T) {
	r := newBodyLimitEngine(8, testOverrides)
	req := httptest.NewRequest(http.MethodPost, "/api/read", strings.NewReader("0123456789"))
	req.ContentLength = -1 // 模拟 chunked：长度未知，必须靠 MaxBytesReader 兜底
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked 超限 body 未被截断: status=%d body=%s", w.Code, w.Body.String())
	}
}
