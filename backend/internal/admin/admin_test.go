// 后台用例站在真路由表上：引擎来自 router.AdminConsole，路由与中间件顺序的唯一真相
// 在 router 包（router_test.go 与 engine.Routes() 双向比对）。本文件因此是**外部测试包**
// admin_test——router import 了 internal/admin，包内测试引 router 会成环；只有外部测试包
// 能引它。代价是用例碰不到 internal/admin 的未导出符号，白盒断言只能改走公开面。
// 留在包内的用例只有两枚：embed_test.go 的 mimeType 与 binding_test.go 的改键 tag 反射钉
// ——都只关本包的局部规则（内嵌资产的 MIME、自己 struct 的 binding），与路由表无关。
package admin_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"ebook-server/internal/admin"
	"ebook-server/model"
	pubjwt "ebook-server/pkg/jwt"
	"ebook-server/pkg/testdb"
	"ebook-server/repository"
	"ebook-server/router"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// 后台会话的测试配置：凭据与密钥都在这里，签发/伪造 token 的用例共用同一份，
// 免得「setup 里的密钥」与「用例里的密钥」各写一遍。
const (
	adminUsername  = "admin"
	adminPassword  = "pass123"
	adminSecret    = "admin-test-secret"
	adminExpireMin = 30
	adminExpire    = time.Duration(adminExpireMin) * time.Minute
)

// loginFailureLimit 后台登录的失败配额：每个来源 IP 每分钟 5 次（README 的「登录限流」
// 与 ADR-0010 §决策 都把它写在纸面上）。这里按**规格**取值，不引生产的同名常量——
// 生产常量被改动而规格没变，用例就该红。
const loginFailureLimit = 5

// adminTokenClaims 后台 token 声明集在用例里的读法：只按 JSON 形状解析，
// 不引 internal/admin 的未导出类型（那正是外部测试包要避开的那道白盒）。
type adminTokenClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// setup 组装后台测试环境：独立 :memory: 库 + 独立会话配置 + **真后台路由表**。
func setup(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	session := admin.NewSession(admin.SessionConfig{
		Username:  adminUsername,
		Password:  adminPassword,
		JWTSecret: adminSecret,
		ExpireMin: adminExpireMin,
	})

	db := testdb.Open(t)
	commentRepo := repository.NewCommentRepository(db)
	h := admin.NewHandler(
		repository.NewUserRepository(db),
		// 读视图、治理删、统计与改键走与公开表面同一个评论模块：规则只有一份实现
		service.NewCommentService(commentRepo),
		repository.NewLogRepository(db),
		session,
	)

	// 引擎来自真表：Base 中间件链、/admin/api 鉴权挂载、SPA 与 Swagger 路由都在其中。
	// 此前这里手抄了 8 条后台路由（路由表的第三份副本），漏挂中间件不会有任何用例变红。
	return router.AdminConsole(router.AdminDeps{Session: session, Handler: h}), db
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
	// 刻意用既不是默认值、也不是 setup 那 30 分钟的时长：Session 若把 TTL 写死在自己身上，
	// 30 这个数会与写死值重合，用例照绿——只有喂给它一个别の数，才测得出「有效期跟着配置走」。
	const expireMin = 17

	session := admin.NewSession(admin.SessionConfig{
		Username:  adminUsername,
		Password:  adminPassword,
		JWTSecret: adminSecret,
		ExpireMin: expireMin,
	})

	token, err := session.Login(adminUsername, adminPassword)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 解析走公开面：用同一把密钥另建一枚 TokenBox。它的 Parse 自带三条校验（算法白名单、
	// 必须带 exp、签发者必须是 pkg/jwt 那一枚常量），所以「后台把 issuer 写成别的」或
	// 「签出的 token 没有 exp」都在这里当场失败，不必再抄一遍常量做第二次断言。
	claims := &adminTokenClaims{}
	if err := pubjwt.New(adminSecret, expireMin).Parse(token, claims); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if claims.Role != "admin" {
		t.Errorf("role = %q, want admin（后台鉴权认的就是这一枚声明）", claims.Role)
	}
	if got := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time); got != time.Duration(expireMin)*time.Minute {
		t.Errorf("有效期没跟着配置走: got %v, want %d 分钟", got, expireMin)
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
		case "A0403": // 口令不符（本文件其余用例同样按字面码断言）
			verified++
		case "A0241": // 尝试过于频繁
			limited++
		default:
			t.Errorf("unexpected code %q", code)
		}
	}
	if verified != loginFailureLimit {
		t.Errorf("only %d attempts should reach password verification, got %d", loginFailureLimit, verified)
	}
	if limited != burst-loginFailureLimit {
		t.Errorf("expected %d rate-limited responses, got %d", burst-loginFailureLimit, limited)
	}
}

// TestAdminLogin_SuccessNotCounted 成功登录不消耗限流配额：
// 连续多次成功登录（超过 limit）不应触发限流。
func TestAdminLogin_SuccessNotCounted(t *testing.T) {
	r, _ := setup(t)

	for i := 0; i < loginFailureLimit+3; i++ {
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

// seedUser 直接入库一个账号并返回其 uid，只为给评论提供外键指向的真实归属。
func seedUser(t *testing.T, db *gorm.DB, email string) uint {
	t.Helper()
	user := &model.User{Email: email, Password: "hp", Username: "seed", Nickname: "seed"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
	return user.UID
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

	seedComment(t, db, seedUser(t, db, "rehash-mine@example.com"), "u1-mine", "ck1:polluted")
	seedComment(t, db, seedUser(t, db, "rehash-theirs@example.com"), "u2-theirs", "ck1:polluted")
	seedComment(t, db, seedUser(t, db, "rehash-other@example.com"), "u3-other", "ck1:untouched")

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

	// 同一把密钥签发的、role 写错的 token：验签、算法白名单、exp、签发者全都过，
	// 只剩 role 这一道能挡。声明在这里手拼——不借后台 Session 的签发单元，
	// 免得「token 形状是对的」成为被测代码自己的功劳。
	now := time.Now()
	forged, err := pubjwt.New(adminSecret, adminExpireMin).Issue(&adminTokenClaims{
		Role: "not-admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(adminExpire)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    pubjwt.Issuer,
		},
	})
	if err != nil {
		t.Fatalf("issue forged token failed: %v", err)
	}

	resp := perform(r, http.MethodGet, "/admin/api/stats", "", forged)
	if resp["code"] != "A0403" {
		t.Errorf("code = %v, want A0403（role 检查失效？）", resp["code"])
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
