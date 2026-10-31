package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ebook-server/config"
	"ebook-server/model"
	"ebook-server/pkg/errcode"
	pubjwt "ebook-server/pkg/jwt"
	"ebook-server/pkg/testdb"
	"ebook-server/repository"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TestMimeType ES module 必须返回 JS MIME，否则浏览器拒绝执行导致白屏（回归保护）。
func TestMimeType(t *testing.T) {
	cases := []struct{ name, want string }{
		{"assets/index-abc.js", "text/javascript; charset=utf-8"},
		{"assets/app-abc.css", "text/css; charset=utf-8"},
		{"assets/app-abc.json", "application/json; charset=utf-8"},
		{"assets/logo.svg", "image/svg+xml"},
	}
	for _, c := range cases {
		if got := mimeType(c.name, nil); got != c.want {
			t.Errorf("mimeType(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// setup 组装后台测试环境：独立 :memory: 库 + 独立会话配置 + 后台路由。
func setup(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	adminCfg := config.AdminConfig{
		Username:  "admin",
		Password:  "pass123",
		JWTSecret: "admin-test-secret",
		ExpireMin: 30,
	}
	session := NewSession(adminCfg)

	db := testdb.Open(t)
	commentRepo := repository.NewCommentRepository(db)
	h := NewHandler(
		repository.NewUserRepository(db),
		// 读视图、治理删、统计与改键走与公开表面同一个评论模块：规则只有一份实现
		service.NewCommentService(commentRepo),
		repository.NewLogRepository(db),
		session,
	)

	r := gin.New()
	adm := r.Group("/admin")
	{
		adm.POST("/api/login", h.Login)
		api := adm.Group("/api")
		api.Use(session.AuthMiddleware())
		{
			api.GET("/stats", h.Stats)
			api.GET("/users", h.ListUsers)
			api.GET("/users/:uid", h.GetUser)
			api.GET("/comments", h.ListComments)
			api.DELETE("/comments/:id", h.DeleteComment)
			api.POST("/comments/rehash", h.RehashComments)
			api.GET("/logs", h.ListLogs)
		}
	}
	return r, db
}

// perform 执行请求并返回响应体。
func perform(r *gin.Engine, method, path, body, token string) map[string]interface{} {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp
}

func adminLogin(t *testing.T, r *gin.Engine) string {
	t.Helper()
	resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"pass123"}`, "")
	if resp["code"] != "00000" {
		t.Fatalf("login failed: %v", resp)
	}
	return resp["data"].(map[string]interface{})["token"].(string)
}

func TestAdminLogin_Success(t *testing.T) {
	r, _ := setup(t)
	if tok := adminLogin(t, r); tok == "" {
		t.Error("expected non-empty admin token")
	}
}

// TestAdminLogin_BadJSONUsesSharedTranslation 绑定失败与公开表面走同一个翻译出口：
// 返回具体缺了哪个字段，而不是笼统的「参数错误」，也不把 validator 英文原文递给调用方。
func TestAdminLogin_BadJSONUsesSharedTranslation(t *testing.T) {
	r, _ := setup(t)

	resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":""}`, "")
	if resp["code"] != "A0400" {
		t.Fatalf("code = %v, want A0400 (%v)", resp["code"], resp)
	}
	if resp["error"] != "请填写后台登录名" {
		t.Fatalf("error = %v, want 共享翻译的中文文案", resp["error"])
	}
}

// TestSession_IssuedTokenClaims 后台 token 的签发者与有效期都取自 pkg/jwt 那一枚单元。
//
// Session 既不再自存一份 expire、也不再自写 issuer 字面量：这两条「与用户端一致」的前提
// 必须由 pkg/jwt 提供，否则改一边忘一边不会有任何地方报错——有效期接错就会把 30 分钟
// 签成立刻过期或签成永不过期，而 issuer 分叉会让两端 token 的校验规则各说各话。
func TestSession_IssuedTokenClaims(t *testing.T) {
	session := NewSession(config.AdminConfig{
		Username:  "admin",
		Password:  "pass123",
		JWTSecret: "admin-test-secret",
		ExpireMin: 30,
	})

	token, err := session.Login("admin", "pass123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	claims := &adminClaims{}
	if err := session.tokens.Parse(token, claims); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if claims.Issuer != pubjwt.Issuer {
		t.Errorf("签发者不是 pkg/jwt 那一枚常量: got %q want %q", claims.Issuer, pubjwt.Issuer)
	}
	if got := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time); got != 30*time.Minute {
		t.Errorf("有效期没跟着配置走: got %v want 30m", got)
	}
}

func TestAdminLogin_WrongPassword(t *testing.T) {
	r, _ := setup(t)
	resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"wrong"}`, "")
	if resp["code"] != "A0403" {
		t.Errorf("expected A0403, got %v", resp["code"])
	}
}

// TestAdminLogin_RateLimit 失败尝试限流（ADR-0010 验收项）：
// 连续 5 次错误密码后第 6 次起返回 A0241；已锁定窗口内即使密码正确也被拒。
func TestAdminLogin_RateLimit(t *testing.T) {
	r, _ := setup(t)

	for i := 1; i <= 6; i++ {
		resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"wrong"}`, "")
		if i <= 5 {
			if resp["code"] != "A0403" {
				t.Errorf("attempt #%d expected A0403, got %v", i, resp["code"])
			}
		} else if resp["code"] != "A0241" {
			t.Errorf("attempt #%d expected A0241 (rate limited), got %v", i, resp["code"])
		}
	}

	// 已锁定窗口内：正确密码同样被拒（A0241）
	resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"pass123"}`, "")
	if resp["code"] != "A0241" {
		t.Errorf("locked window should reject correct password with A0241, got %v", resp["code"])
	}
}

// TestAdminLogin_RateLimitIsAtomicUnderBurst 并发爆发不能绕过失败限流：
// 「Peek → 校验 → Allow」若分成三步独立加锁，一批同时到达的请求会全部通过 Peek，
// 于是一次爆发可以试任意多次口令。临界区收进 Handler 后，恰好只有前 limit 次
// 真正做了密码校验。
func TestAdminLogin_RateLimitIsAtomicUnderBurst(t *testing.T) {
	const burst = 20
	r, _ := setup(t)

	start := make(chan struct{})
	codes := make(chan string, burst)
	var wg sync.WaitGroup
	for i := 0; i < burst; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // 尽量让请求同时进入
			resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"wrong"}`, "")
			code, _ := resp["code"].(string)
			codes <- code
		}()
	}
	close(start)
	wg.Wait()
	close(codes)

	verified, limited := 0, 0
	for code := range codes {
		switch code {
		case adminAuthFailureCode:
			verified++
		case errcode.AttemptTooMany:
			limited++
		default:
			t.Errorf("unexpected code %q", code)
		}
	}
	if verified != loginLimiterLimit {
		t.Errorf("only %d attempts should reach password verification, got %d", loginLimiterLimit, verified)
	}
	if limited != burst-loginLimiterLimit {
		t.Errorf("expected %d rate-limited responses, got %d", burst-loginLimiterLimit, limited)
	}
}

// TestAdminLogin_SuccessNotCounted 成功登录不消耗限流配额：
// 连续多次成功登录（超过 limit）不应触发限流。
func TestAdminLogin_SuccessNotCounted(t *testing.T) {
	r, _ := setup(t)

	for i := 0; i < loginLimiterLimit+3; i++ {
		resp := perform(r, http.MethodPost, "/admin/api/login", `{"username":"admin","password":"pass123"}`, "")
		if resp["code"] != "00000" {
			t.Fatalf("successful login #%d should not be rate-limited, got %v", i+1, resp["code"])
		}
	}
}

func TestAdminAuth_RequiresToken(t *testing.T) {
	r, _ := setup(t)
	resp := perform(r, http.MethodGet, "/admin/api/stats", "", "")
	if resp["code"] != "A0403" {
		t.Errorf("expected A0403 without token, got %v", resp["code"])
	}
}

func TestAdminAuth_RejectsPublicJWT(t *testing.T) {
	r, _ := setup(t)
	// 公开用户 JWT 用自己的那把密钥签发；管理端是 admin.JWTSecret，跨密钥签名应被拒。
	pub, err := pubjwt.New("public-secret", 30).IssueUser(1, "someone")
	if err != nil {
		t.Fatalf("generate public token failed: %v", err)
	}
	resp := perform(r, http.MethodGet, "/admin/api/stats", "", pub)
	if resp["code"] != "A0403" {
		t.Errorf("expected public JWT rejected by admin auth, got %v", resp["code"])
	}
}

func TestAdminStats(t *testing.T) {
	r, db := setup(t)

	// 造一个用户，验证 /api/stats 复用 repository 能取到真实计数。
	users := repository.NewUserRepository(db)
	if err := users.Create(testCtx, &model.User{
		Email: "admin-stats@example.com", Password: "x", Username: "admin-stats",
	}); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	tok := adminLogin(t, r)
	resp := perform(r, http.MethodGet, "/admin/api/stats", "", tok)
	if resp["code"] != "00000" {
		t.Fatalf("stats failed: %v", resp)
	}
	data := resp["data"].(map[string]interface{})
	if int(data["users"].(float64)) < 1 {
		t.Errorf("expected users>=1, got %v", data["users"])
	}
}

func TestAdminListLogs(t *testing.T) {
	r, db := setup(t)

	// 造一条操作日志，验证 /admin/api/logs 能查到并返回。
	logs := repository.NewLogRepository(db)
	if err := logs.Create(testCtx, &model.OperationLog{OperationRecord: model.OperationRecord{Method: "GET", Path: "/api/comments", ResponseCode: 200}}); err != nil {
		t.Fatalf("create log failed: %v", err)
	}

	tok := adminLogin(t, r)
	resp := perform(r, http.MethodGet, "/admin/api/logs", "", tok)
	if resp["code"] != "00000" {
		t.Fatalf("list logs failed: %v", resp)
	}
	data := resp["data"].(map[string]interface{})
	if int(data["total"].(float64)) < 1 {
		t.Errorf("expected at least 1 log, got total=%v", data["total"])
	}
}

// TestAdminListPagingUsesSiteDefault 后台列表的分页缺省与公开表面是同一个值。
//
// 后台曾自己写了一份 paginate：缺省 20、越界退回 20，与公开的 10 并存，于是
// 「省略 page_size 会拿到几条」没有唯一答案，改规则的人得记着改两处。现在两侧
// 都经 httpx.Page → model.NormalizePage，这里钉住端到端结果而不是钉数值本身：
// 缺省按 model.DefaultPageSize 返回，越界与非数字都退回缺省而不是报错。
// （真实的管理界面显式传 page_size=20，不受这次收敛影响。）
func TestAdminListPagingUsesSiteDefault(t *testing.T) {
	r, db := setup(t)

	seed := model.DefaultPageSize + 5
	for i := 0; i < seed; i++ {
		suffix := strconv.Itoa(i)
		if err := db.Create(&model.User{
			Email: "paging-" + suffix + "@example.com", Password: "x", Username: "u" + suffix,
		}).Error; err != nil {
			t.Fatalf("seed user failed: %v", err)
		}
	}

	tok := adminLogin(t, r)

	for _, q := range []string{"", "?page_size=99999", "?page_size=abc", "?page_size=0"} {
		t.Run("query="+q, func(t *testing.T) {
			resp := perform(r, http.MethodGet, "/admin/api/users"+q, "", tok)
			if resp["code"] != "00000" {
				t.Fatalf("list users failed: %v", resp)
			}
			data := resp["data"].(map[string]interface{})
			if got := len(data["list"].([]interface{})); got != model.DefaultPageSize {
				t.Errorf("list = %d 条, want %d（缺省页大小全站只有一处定义）", got, model.DefaultPageSize)
			}
			if total := int64(data["total"].(float64)); total != int64(seed) {
				t.Errorf("total = %d, want %d（钳制只收窄当页，不该改动总数）", total, seed)
			}
		})
	}

	// 显式的合法页大小原样生效，证明上面的缺省不是「永远返回 10 条」。
	resp := perform(r, http.MethodGet, "/admin/api/users?page_size=3", "", tok)
	data := resp["data"].(map[string]interface{})
	if got := len(data["list"].([]interface{})); got != 3 {
		t.Errorf("page_size=3 → %d 条, want 3", got)
	}
}

// seedComment 直接入库一条归属某用户、挂在某聚合键下的评论。
func seedComment(t *testing.T, db *gorm.DB, userID uint, content, commentKey string) {
	t.Helper()
	if err := db.Create(&model.Comment{
		UserID: userID, Content: content, CommentKey: commentKey,
	}).Error; err != nil {
		t.Fatalf("seed comment failed: %v", err)
	}
}

// countUnder 统计某聚合键下的评论行数（直查库，绕开被测端点）。
func countUnder(t *testing.T, db *gorm.DB, commentKey string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.Comment{}).Where("comment_key = ?", commentKey).Count(&count).Error; err != nil {
		t.Fatalf("count comments failed: %v", err)
	}
	return count
}

// TestAdminRehashComments_CrossUser 后台全局改键跨用户生效，且返回受影响行数。
//
// 这正是它与公开 migrate 端点的唯一区别：桶污染时错键上聚着别人的评论，
// 只动本人行的迁移救不了场。
func TestAdminRehashComments_CrossUser(t *testing.T) {
	r, db := setup(t)

	seedComment(t, db, 1, "u1-mine", "ck1:polluted")
	seedComment(t, db, 2, "u2-theirs", "ck1:polluted")
	seedComment(t, db, 3, "u3-other", "ck1:untouched")

	tok := adminLogin(t, r)
	resp := perform(r, http.MethodPost, "/admin/api/comments/rehash",
		`{"old_key":"ck1:polluted","new_key":"ck1:fixed"}`, tok)
	if resp["code"] != "00000" {
		t.Fatalf("rehash failed: %v", resp)
	}
	data := resp["data"].(map[string]interface{})
	if int64(data["migrated_count"].(float64)) != 2 {
		t.Errorf("expected migrated_count 2, got %v", data["migrated_count"])
	}
	if got := countUnder(t, db, "ck1:polluted"); got != 0 {
		t.Errorf("expected polluted bucket emptied, got %v rows", got)
	}
	if got := countUnder(t, db, "ck1:fixed"); got != 2 {
		t.Errorf("expected 2 rows under new key, got %v", got)
	}
	if got := countUnder(t, db, "ck1:untouched"); got != 1 {
		t.Errorf("expected unrelated bucket untouched, got %v rows", got)
	}
}

// TestAdminRehashComments_SameKey 新旧相同返回 A0305，而不是谎报「改了 0 行」。
func TestAdminRehashComments_SameKey(t *testing.T) {
	r, _ := setup(t)
	tok := adminLogin(t, r)

	resp := perform(r, http.MethodPost, "/admin/api/comments/rehash",
		`{"old_key":"ck1:same","new_key":"ck1:same"}`, tok)
	if resp["code"] != "A0305" {
		t.Errorf("expected A0305, got %v", resp["code"])
	}
}

// TestAdminRehashComments_RejectsEmptyOldKey 空 old_key 必须被拒。
//
// 公开的 migrate 允许空旧键（把本人未换键的历史行收进正确桶），这里绝不允许：
// 不带用户过滤的空旧键等于把全站所有未换键的行扫进同一个桶，是制造污染而非修复。
func TestAdminRehashComments_RejectsEmptyOldKey(t *testing.T) {
	r, _ := setup(t)
	tok := adminLogin(t, r)

	resp := perform(r, http.MethodPost, "/admin/api/comments/rehash",
		`{"old_key":"","new_key":"ck1:target"}`, tok)
	if resp["code"] != "A0400" {
		t.Errorf("expected A0400 for empty old_key, got %v", resp["code"])
	}
}

// TestAdminRehashComments_RequiresToken 全局改键必须走后台鉴权（ADR-0010）。
func TestAdminRehashComments_RequiresToken(t *testing.T) {
	r, _ := setup(t)

	resp := perform(r, http.MethodPost, "/admin/api/comments/rehash",
		`{"old_key":"ck1:a","new_key":"ck1:b"}`, "")
	if resp["code"] != "A0403" {
		t.Errorf("expected A0403 without admin token, got %v", resp["code"])
	}
}

// TestAdminToken_WrongRoleRejected 签名正确但 role 不是 admin 的 token 必须被拒。
//
// 后台的隔离靠「另一把密钥 + role 声明」两道；只测密钥那道，等于默认第二道不会漏。
// 这条以前没有任何用例——Session.validate 里的 role 检查无人盯。
func TestAdminToken_WrongRoleRejected(t *testing.T) {
	r, _ := setup(t)
	session := NewSession(config.AdminConfig{
		Username: "admin", Password: "pass123", JWTSecret: "admin-test-secret", ExpireMin: 30,
	})

	// 同一把密钥签发的、role 写错的 token：验签会过，只能靠 role 检查挡住
	forged, err := session.tokens.Issue(&adminClaims{Role: "not-admin"})
	if err != nil {
		t.Fatalf("issue forged token failed: %v", err)
	}

	resp := perform(r, http.MethodGet, "/admin/api/stats", "", forged)
	if resp["code"] != "A0403" {
		t.Errorf("code = %v, want A0403（role 检查失效？）", resp["code"])
	}
}

// TestRehashRequestBindingMatchesColumnLimit 钉住后台改键的 binding 与列约束同源。
//
// tag 里的 200 与 model.MaxCommentKeyLength 是同一列的同一个上限，抄在两处就会漂；
// 漂了的后果是把评论迁到一个自己再也提交不出、也就永远读不回来的键上。
func TestRehashRequestBindingMatchesColumnLimit(t *testing.T) {
	cases := []struct{ field, wantRequired string }{
		{"OldKey", "required"},
		{"NewKey", "required"},
	}
	typ := reflect.TypeOf(rehashRequest{})

	for _, tc := range cases {
		field, ok := typ.FieldByName(tc.field)
		if !ok {
			t.Fatalf("rehashRequest has no field %s", tc.field)
		}
		tag := string(field.Tag.Get("binding"))
		if !strings.Contains(tag, tc.wantRequired) {
			t.Errorf("%s binding = %q, want it to contain %q", tc.field, tag, tc.wantRequired)
		}
		if maxOf := bindingMaxFromTag(t, tag); maxOf != model.MaxCommentKeyLength {
			t.Errorf("%s binding max = %d, want model.MaxCommentKeyLength %d", tc.field, maxOf, model.MaxCommentKeyLength)
		}
	}
}

// TestAdminRehash_RejectsMissingKeys 两键都缺时回 A0400（缺参数），而不是 A0305（新旧相同）。
//
// 判定顺序是刻意的：先问「键合不合规」，再问「新旧是否相同」——退化输入不该被
// 报成一次「恰好没变化的迁移」，那会把运维引向换个不同的键再试而不是把参数传全。
func TestAdminRehash_RejectsMissingKeys(t *testing.T) {
	r, _ := setup(t)
	tok := adminLogin(t, r)

	for _, body := range []string{`{}`, `{"old_key":"","new_key":""}`} {
		resp := perform(r, http.MethodPost, "/admin/api/comments/rehash", body, tok)
		if resp["code"] != "A0400" {
			t.Errorf("body=%s code = %v, want A0400", body, resp["code"])
		}
	}
}

// bindingMaxFromTag 从 binding tag（如 "required,max=200"）里取出 max 的数值。
func bindingMaxFromTag(t *testing.T, tag string) int {
	t.Helper()
	for _, part := range strings.Split(tag, ",") {
		if value, found := strings.CutPrefix(strings.TrimSpace(part), "max="); found {
			n, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("bad max in binding tag %q: %v", tag, err)
			}
			return n
		}
	}
	t.Fatalf("no max= in binding tag %q", tag)
	return 0
}
