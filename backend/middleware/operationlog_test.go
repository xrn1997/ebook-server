package middleware

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ebook-server/model"
	"ebook-server/pkg/jwt"

	"github.com/gin-gonic/gin"
)

// fakeRecorder 记录最近一次交给审计模块的操作事实与上下文状态。
//
// 上下文本身会在 Record 返回后由调用方 cancel；因此快照 Err/Deadline，而不是
// 事后再读 ctx.Err()（那时看到的是正常的收尾取消）。
type fakeRecorder struct {
	record      *model.OperationRecord
	ctxErr      error
	hasDeadline bool
	err         error
}

func (f *fakeRecorder) Record(ctx context.Context, record model.OperationRecord) error {
	f.ctxErr = ctx.Err()
	_, f.hasDeadline = ctx.Deadline()
	f.record = &record
	return f.err
}

func TestOperationLog_PersistsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeRecorder{}
	r := gin.New()
	r.Use(OperationLog(w))
	r.GET("/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	req.Header.Set("User-Agent", "test-agent")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if w.record == nil {
		t.Fatal("expected operation log to be written")
	}
	if w.record.Method != http.MethodGet || w.record.Path != "/hello" {
		t.Errorf("unexpected method/path: %s %s", w.record.Method, w.record.Path)
	}
	if w.record.ResponseCode != http.StatusOK {
		t.Errorf("unexpected response code: %d", w.record.ResponseCode)
	}
	if w.record.UserAgent != "test-agent" {
		t.Errorf("expected UserAgent recorded, got %q", w.record.UserAgent)
	}
	// 未登录的请求没有「谁做的」：不得凭空写出一个身份
	if w.record.UserID != 0 || w.record.Username != "" {
		t.Errorf("未认证请求不该带出身份：uid=%d username=%q", w.record.UserID, w.record.Username)
	}
}

// TestOperationLog_CarriesSignedInUser 当前身份也是交给审计模块的一条事实。
//
// ADR-0018 §1 让中间件只交出「协议层能拿到的事实」，其中包含 JWTAuth 注入的用户。
// 这两行接线一旦断掉，业务响应照旧 200、审计也照常入库，只有「谁做的」静默变空——
// 后台按 user_id 筛与 /api/logs/my 都会查不到东西，而没有任何一处会报错。
func TestOperationLog_CarriesSignedInUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeRecorder{}
	box := jwt.New("oplog-test-secret", 60)
	token, err := box.IssueUser(42, "audited-user")
	if err != nil {
		t.Fatalf("IssueUser: %v", err)
	}

	r := gin.New()
	// 顺序与生产一致：审计在外层，身份在 c.Next() 期间由鉴权中间件注入
	r.Use(OperationLog(w))
	authed := r.Group("/api")
	authed.Use(JWTAuth(box))
	authed.GET("/me", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if w.record == nil {
		t.Fatal("expected operation log to be written")
	}
	if w.record.UserID != 42 {
		t.Errorf("record.UserID = %d, want 42", w.record.UserID)
	}
	if w.record.Username != "audited-user" {
		t.Errorf("record.Username = %q, want audited-user", w.record.Username)
	}
}

// TestOperationLog_ForwardsEveryPath 中间件不做路径过滤：规则属于日志模块。
//
// 挂载点（只挂 /api）决定「哪些表面接入审计」，模块里的 Record 决定「哪些路径
// 算一次操作」。中间件若也过滤一遍，两处规则就会各自演化——这里钉住它只转发。
func TestOperationLog_ForwardsEveryPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeRecorder{}
	r := gin.New()
	r.Use(OperationLog(w))
	for _, path := range []string{"/api/comments", "/admin/api/stats", "/health"} {
		r.GET(path, func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	}

	for _, path := range []string{"/api/comments", "/admin/api/stats", "/health"} {
		w.record = nil
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		if w.record == nil || w.record.Path != path {
			t.Errorf("middleware 应把 %s 交给日志模块，got %+v", path, w.record)
		}
	}
}

// TestOperationLog_DetachesAuditContextFromCancellation 客户端断开时审计仍要落库。
//
// 请求取消后 c.Request.Context() 已失效，直接传给数据库会让 INSERT 必然失败；
// 而断开的请求恰恰是审计最该保留的记录。中间件应换成一个带短超时的独立上下文。
func TestOperationLog_DetachesAuditContextFromCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeRecorder{}
	r := gin.New()
	r.Use(OperationLog(w))
	r.GET("/api/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil).WithContext(reqCtx)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if w.record == nil {
		t.Fatal("audit record missing")
	}
	if w.ctxErr != nil {
		t.Fatalf("audit context is already canceled: %v", w.ctxErr)
	}
	if !w.hasDeadline {
		t.Fatal("audit context should carry a short timeout")
	}
}

// TestOperationLog_RecordsOnPanic handler panic 时审计事实也不能随栈展开丢掉，
// 且必须记成业务失败（router.Public 在审计内侧再挂一层 Recovery）。
func TestOperationLog_RecordsOnPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeRecorder{}
	r := gin.New()
	r.Use(Recovery()) // 引擎级兜底，与生产一致
	api := r.Group("/api")
	api.Use(OperationLog(w))
	api.Use(Recovery()) // 审计内侧：panic 先落成 C0500 信封，审计才看得到业务码
	api.GET("/boom", func(c *gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/boom", nil))

	if w.record == nil {
		t.Fatal("panic request should still produce an audit record")
	}
	if w.record.Path != "/api/boom" {
		t.Errorf("audit path = %q, want /api/boom", w.record.Path)
	}
	if w.record.ErrorCode != "C0500" {
		t.Errorf("panic 审计业务码 = %q, want C0500（Recovery 必须在审计内侧）", w.record.ErrorCode)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want 200（信封约定）", rec.Code)
	}
}

// TestOperationLog_WriteErrorDoesNotBlock 写入失败只告警不阻断请求。
func TestOperationLog_WriteErrorDoesNotBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeRecorder{err: errors.New("boom")}
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

// TestOperationLog_CapturesBusinessCode HTTP 恒 200，但应从响应信封提取业务码/文案入审计。
func TestOperationLog_CapturesBusinessCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &fakeRecorder{}
	r := gin.New()
	r.Use(OperationLog(w))
	r.GET("/api/auth/send-code", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": "C0500", "error": "发送验证码失败"})
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/auth/send-code", nil))
	if w.record == nil {
		t.Fatal("expected operation log written")
	}
	if w.record.ErrorCode != "C0500" {
		t.Errorf("expected ErrorCode C0500, got %q", w.record.ErrorCode)
	}
	if w.record.ErrorMessage != "发送验证码失败" {
		t.Errorf("expected ErrorMessage captured, got %q", w.record.ErrorMessage)
	}
	// 响应必须仍能如实转发（拦截后 body 可见）
	if w.record.ResponseCode != http.StatusOK {
		t.Errorf("expected 200, got %d", w.record.ResponseCode)
	}
}

// TestRecordWriter_BoundsJSONCapture 审计只保留至多 maxEnvelopeBytes 的 JSON，
// 但真实响应必须完整转发。
func TestRecordWriter_BoundsJSONCapture(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	bw := &recordWriter{ResponseWriter: c.Writer}
	bw.Header().Set("Content-Type", "application/json; charset=utf-8")

	payload := bytes.Repeat([]byte("x"), 10<<10)
	if _, err := bw.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if bw.body.Len() != maxEnvelopeBytes {
		t.Fatalf("captured %d bytes, want %d", bw.body.Len(), maxEnvelopeBytes)
	}
	if rec.Body.Len() != len(payload) {
		t.Fatalf("forwarded %d bytes, want %d（截断只能作用于审计缓冲）", rec.Body.Len(), len(payload))
	}
}

// TestRecordWriter_SkipsNonJSON 非 JSON 响应没有信封，不占用缓冲。
func TestRecordWriter_SkipsNonJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	bw := &recordWriter{ResponseWriter: c.Writer}
	bw.Header().Set("Content-Type", "image/png")

	if _, err := bw.Write(bytes.Repeat([]byte("x"), 1024)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if bw.body.Len() != 0 {
		t.Fatalf("captured non-JSON body: %d bytes", bw.body.Len())
	}
}
