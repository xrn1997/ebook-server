package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ebook-server/config"
	"ebook-server/handler"
	"ebook-server/internal/admin"
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/jwt"
	"ebook-server/pkg/testdb"
	"ebook-server/pkg/upload"
	"ebook-server/repository"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 路由表此前住在 main() 里：没有 interface，因而没有用例。handler 测试各自另抄一份
// 「它需要的那几行路由」，于是真实装配漏挂中间件时没人能发现——本文件跑的是真表。

// denyAll 永远拒绝的 jwt.Parser adapter（这个 seam 上的第二个 adapter）。
//
// 有了它，「未登录被挡」才是在测中间件挂没挂，而不是恰好因为 token 有效而通过。
type denyAll struct{}

func (denyAll) ParseUser(string) (*jwt.Claims, error) { return nil, errors.New("denied") }

// discardMailer 满足 service.Mailer：路由用例不关心邮件内容。
type discardMailer struct{}

func (discardMailer) SendCode(string, string) error { return nil }

// tableDeps 一张真表所需的东西：路由依赖 + 后面组装后台表要复用的服务与连接。
type tableDeps struct {
	deps     Deps
	access   *jwt.TokenBox
	comments *service.CommentService
	userRepo *repository.UserRepository
	logRepo  *repository.LogRepository
	db       *gorm.DB
	// uploadRoot 头像存储的根目录（upload.dir）：断言 /uploads 只放出头像那一层。
	uploadRoot string
}

// newTableDeps 用真依赖（:memory: SQLite + 真 service）组装一张表。
func newTableDeps(t *testing.T) tableDeps {
	t.Helper()

	db := testdb.Open(t)
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewRefreshTokenRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	logRepo := repository.NewLogRepository(db)

	access := jwt.New("router-test-secret", 60)
	verifications := service.NewVerificationCodes(code.NewStore(), discardMailer{})
	uploadRoot := t.TempDir()
	uploadStore := upload.New(uploadRoot, "")

	credentials := service.NewCredentials(userRepo, tokenRepo)
	authService := service.NewAuthService(userRepo, tokenRepo, verifications, access, credentials)
	profileService := service.NewProfileService(userRepo, uploadStore)
	accountService := service.NewAccountService(userRepo, tokenRepo, commentRepo, verifications, profileService)
	commentService := service.NewCommentService(commentRepo)

	return tableDeps{
		deps: Deps{
			Handlers: Handlers{
				Auth:    handler.NewAuthHandler(authService),
				User:    handler.NewUserHandler(profileService, authService),
				Account: handler.NewAccountHandler(accountService),
				Comment: handler.NewCommentHandler(commentService),
				Log:     handler.NewLogHandler(service.NewLogService(logRepo)),
				Upload:  handler.NewUploadHandler(uploadStore),
			},
			Access:    access,
			AvatarDir: uploadStore.AvatarDir(),
		},
		access:     access,
		comments:   commentService,
		userRepo:   userRepo,
		logRepo:    logRepo,
		db:         db,
		uploadRoot: uploadRoot,
	}
}

// codeOf 走一次请求并取回信封里的业务码。
func codeOf(t *testing.T, engine *gin.Engine, method, path, body, token string) string {
	t.Helper()

	var reader *strings.Reader = strings.NewReader(body)
	req, err := http.NewRequest(method, path, reader)
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %s %s response failed: %v (%s)", method, path, err, w.Body.String())
	}
	codeVal, _ := resp["code"].(string)
	return codeVal
}

// declaredRoute 公开表的一条登记项。
//
// pattern 是 gin 注册的模式（engine.Routes() 给出的就是它），sample 是符合该模式、
// 能真发一次请求的具体路径。
//
// 全部用例共用这一张表，是因为清单一旦抄成两份就有两种失效方式：新加的一路只写进
// 一份，另一份的断言就永远碰不到它；或者两份悄悄漂移成不同的答案。AGENTS.md 承诺
// 「漏挂鉴权会在这里失败」，只有当「每一路都必须登记一次、且登记时必答要不要登录」
// 才成立——所以下面有一条用例拿这张表和真实 engine.Routes() 双向比对。
type declaredRoute struct {
	method   string
	pattern  string
	sample   string
	signedIn bool
}

// publicTable 公开 API 表面的全部业务路由。
var publicTable = []declaredRoute{
	{http.MethodGet, "/health", "/health", false},
	{http.MethodGet, "/api/comments", "/api/comments", false},
	{http.MethodPost, "/api/auth/send-code", "/api/auth/send-code", false},
	{http.MethodPost, "/api/auth/register", "/api/auth/register", false},
	{http.MethodPost, "/api/auth/login", "/api/auth/login", false},
	{http.MethodPost, "/api/auth/refresh", "/api/auth/refresh", false},
	{http.MethodPost, "/api/auth/forgot-password/send-code", "/api/auth/forgot-password/send-code", false},
	{http.MethodPost, "/api/auth/forgot-password/reset", "/api/auth/forgot-password/reset", false},

	{http.MethodPost, "/api/auth/logout", "/api/auth/logout", true},
	{http.MethodGet, "/api/users/me", "/api/users/me", true},
	{http.MethodPut, "/api/users/me", "/api/users/me", true},
	{http.MethodPut, "/api/users/me/password", "/api/users/me/password", true},
	{http.MethodGet, "/api/users/me/data", "/api/users/me/data", true},
	{http.MethodPost, "/api/users/me/deletion/send-code", "/api/users/me/deletion/send-code", true},
	{http.MethodPost, "/api/users/me/deletion", "/api/users/me/deletion", true},
	{http.MethodPost, "/api/uploads/avatar", "/api/uploads/avatar", true},
	{http.MethodPost, "/api/comments", "/api/comments", true},
	{http.MethodPost, "/api/comments/migrate", "/api/comments/migrate", true},
	{http.MethodGet, "/api/comments/my", "/api/comments/my", true},
	{http.MethodDelete, "/api/comments/:id", "/api/comments/1", true},
	{http.MethodGet, "/api/logs/my", "/api/logs/my", true},
}

// mountedRoutes 非业务路由：静态挂载与可选的文档路由，回的不是统一信封，
// 所以不参与鉴权断言，只参与「有没有未登记的路由」比对。
//
// 静态挂载的模式写成 <挂载前缀>/*filepath（gin 把整段前缀一起注册进来），
// 且 GET/HEAD 各一条——前缀从 upload 模块取，不在这儿再抄一遍 /uploads。
func mountedRoutes(apiDocs bool) []string {
	static := upload.AvatarURLPrefix + "/*filepath"
	mounted := []string{
		routeKey(http.MethodGet, static),
		routeKey(http.MethodHead, static),
	}
	if apiDocs {
		mounted = append(mounted, routeKey(http.MethodGet, "/api-docs/*any"))
	}
	return mounted
}

func routeKey(method, path string) string { return method + " " + path }

// TestPublicTable_EveryRouteIsDeclared 真实路由表与本表必须双向相等。
//
// 这条用例是「漏挂鉴权会在这里失败」那句话的兑现处：新加一路而不在这里登记一次
// （登记就得回答要不要登录），这里会直接失败。同时断言后台路径不会出现在公开
// 引擎上——ADR-0010 的隔离靠两张表各自独立，最怕的恰恰是有人图省事把 /admin
// 顺手挂到公开表面。
func TestPublicTable_EveryRouteIsDeclared(t *testing.T) {
	td := newTableDeps(t)
	engine := Public(td.deps) // APIDocs 默认 false

	declared := map[string]bool{}
	for _, r := range publicTable {
		declared[routeKey(r.method, r.pattern)] = true
	}
	for _, key := range mountedRoutes(false) {
		declared[key] = true
	}

	actual := map[string]bool{}
	for _, ri := range engine.Routes() {
		key := routeKey(ri.Method, ri.Path)
		actual[key] = true
		if !declared[key] {
			t.Errorf("路由 %s 未登记在 publicTable：新加的一路必须显式回答「要不要登录」", key)
		}
		if strings.HasPrefix(ri.Path, "/admin") {
			t.Errorf("后台路由 %s 出现在公开引擎上（ADR-0010：两张表必须独立）", key)
		}
	}
	for key := range declared {
		if !actual[key] {
			t.Errorf("登记过的路由 %s 在真实表里找不到：注册被删了还是前缀改了？", key)
		}
	}
}

// TestPublicTable_HealthCheck 健康检查无需认证即可通（它不在鉴权组里，也不写信封外的响应）。
func TestPublicTable_HealthCheck(t *testing.T) {
	td := newTableDeps(t)
	engine := Public(td.deps)

	if got := codeOf(t, engine, http.MethodGet, "/health", "", ""); got != "00000" {
		t.Errorf("/health code = %s, want 00000", got)
	}
}

// TestPublicTable_ProtectedRoutesRejectAnonymous 每条需要登录的路都在鉴权组里。
//
// 断言的是「这张表挂了什么」而不是某个 handler 的行为，所以用 denyAll：
// 任何一路漏挂中间件，它会回出 A0400/00000 而不是 A0230，用例即失败。
func TestPublicTable_ProtectedRoutesRejectAnonymous(t *testing.T) {
	td := newTableDeps(t)
	td.deps.Access = denyAll{}
	engine := Public(td.deps)

	for _, r := range publicTable {
		if !r.signedIn {
			continue
		}
		t.Run(r.method+" "+r.sample, func(t *testing.T) {
			if got := codeOf(t, engine, r.method, r.sample, "{}", "forged-token"); got != "A0230" {
				t.Errorf("code = %s, want A0230（该路未挂鉴权？）", got)
			}
		})
	}
}

// TestPublicTable_PublicRoutesStayOpen 公开的那几路不该要求登录。
func TestPublicTable_PublicRoutesStayOpen(t *testing.T) {
	td := newTableDeps(t)
	td.deps.Access = denyAll{}
	engine := Public(td.deps)

	for _, r := range publicTable {
		if r.signedIn {
			continue
		}
		t.Run(r.method+" "+r.sample, func(t *testing.T) {
			body := ""
			if r.method == http.MethodPost {
				body = "{}"
			}
			if got := codeOf(t, engine, r.method, r.sample, body, ""); got == "A0230" {
				t.Error("公开端点回了 A0230——被错误地挂上了鉴权")
			}
		})
	}
}

// TestPublicTable_UploadsServesOnlyAvatars /uploads/avatar 放出头像，同级的其他文件不放。
//
// 挂载点曾经是整个 upload.dir（即 config 的 upload.dir，相对服务端工作目录解析），
// 于是 GET /uploads/.env 与 GET /uploads/ebook.db 都回 200——把后台口令、JWT 密钥
// 与全部评论一次性交出去。这里两头都断：头像必须仍然可取（收窄别把功能改坏），
// 兄弟文件必须取不到。
func TestPublicTable_UploadsServesOnlyAvatars(t *testing.T) {
	td := newTableDeps(t)
	engine := Public(td.deps)

	writeFile(t, filepath.Join(td.uploadRoot, ".env"), "JWT_SECRET=leaked\n")
	writeFile(t, filepath.Join(td.deps.AvatarDir, "pic.png"), "png-bytes")

	if code, body := bodyOf(t, engine, "/uploads/avatar/pic.png"); code != http.StatusOK || body != "png-bytes" {
		t.Errorf("头像取回 %d %q, want 200 png-bytes（挂载点没对上 upload.Store.AvatarDir()）", code, body)
	}
	if code, body := bodyOf(t, engine, "/uploads/.env"); code == http.StatusOK || strings.Contains(body, "leaked") {
		t.Errorf("/uploads/.env 竟然可读（%d）：静态挂载越出了头像目录", code)
	}
}

// writeFile 写一个临时文件，失败即终止用例。
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入 %s 失败: %v", path, err)
	}
}

// bodyOf 走一次 GET，回状态码与响应体。
func bodyOf(t *testing.T, engine *gin.Engine, path string) (int, string) {
	t.Helper()

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w.Code, w.Body.String()
}

// TestPublicTable_AcceptsTokenFromItsOwnKey 注入的 token 单元签发的 token 能过鉴权。
func TestPublicTable_AcceptsTokenFromItsOwnKey(t *testing.T) {
	td := newTableDeps(t)
	engine := Public(td.deps)

	token, err := td.access.IssueUser(7, "router-user")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}
	if got := codeOf(t, engine, http.MethodGet, "/api/comments/my", "", token); got == "A0230" {
		t.Errorf("合法 token 被挡：code = %s", got)
	}
}

// TestAdminConsole_RequiresAdminToken 后台 API 必须带后台 token；用户 token 不算。
func TestAdminConsole_RequiresAdminToken(t *testing.T) {
	td := newTableDeps(t)

	session := admin.NewSession(config.AdminConfig{
		Username:  "admin",
		Password:  "pass123",
		JWTSecret: "admin-router-secret",
		ExpireMin: 30,
	})
	engine := AdminConsole(AdminDeps{
		Session: session,
		Handler: admin.NewHandler(td.userRepo, td.comments, td.logRepo, session),
	})

	if got := codeOf(t, engine, http.MethodGet, "/admin/api/stats", "", ""); got != "A0403" {
		t.Errorf("匿名访问后台 code = %s, want A0403", got)
	}

	userToken, err := td.access.IssueUser(1, "someone")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}
	if got := codeOf(t, engine, http.MethodGet, "/admin/api/stats", "", userToken); got != "A0403" {
		t.Errorf("用户 token 打后台 code = %s, want A0403（两把密钥必须互不认账）", got)
	}

	adminToken, err := session.Login("admin", "pass123")
	if err != nil {
		t.Fatalf("后台登录失败: %v", err)
	}
	if got := codeOf(t, engine, http.MethodGet, "/admin/api/stats", "", adminToken); got != "00000" {
		t.Errorf("后台 token code = %s, want 00000", got)
	}
}

// okParser 永远放行的 middleware.Parser adapter：给出一个不存在但非 0 的身份。
type okParser struct{}

func (okParser) ParseUser(string) (*jwt.Claims, error) {
	return &jwt.Claims{UserID: 424242, Username: "pass-through"}, nil
}

// TestPublicTable_ProtectedRoutesCarryIdentity 反方向钉住鉴权中间件在位。
//
// 只用「永远拒绝」的 adapter 断 A0230 挡不住漏挂鉴权：handler 里的 CurrentUser
// 自己也会回 A0230，两种故障长得一模一样，断言就成了同义反复。换成「永远放行」的
// adapter 后，中间件一旦被摘掉，context 里就没有 user_id，handler 回 A0230——
// 这条断言只有「中间件在位并把身份写进 context」才能通过。
func TestPublicTable_ProtectedRoutesCarryIdentity(t *testing.T) {
	td := newTableDeps(t)
	td.deps.Access = okParser{}
	engine := Public(td.deps)

	for _, r := range publicTable {
		if !r.signedIn {
			continue
		}
		t.Run(r.method+" "+r.sample, func(t *testing.T) {
			if got := codeOf(t, engine, r.method, r.sample, "{}", "any-token"); got == "A0230" {
				t.Errorf("放行 adapter 下仍回 A0230 —— 该路漏挂了 JWTAuth，只靠 handler 自查兜着")
			}
		})
	}
}

// TestPublicTable_APIDocsSwitch 文档路由只随 api_docs 开关出现。
//
// 公开端口默认不提供 Swagger（防接口清单泄露），所以「关着时确实没有」与
// 「开着时确实在」两头都要断：只断后者的话，把开关写反也测不出来。
func TestPublicTable_APIDocsSwitch(t *testing.T) {
	key := http.MethodGet + " /api-docs/*any"

	off := map[string]bool{}
	for _, ri := range Public(newTableDeps(t).deps).Routes() {
		off[routeKey(ri.Method, ri.Path)] = true
	}
	if off[key] {
		t.Error("api_docs.enabled=false 时公开端口仍提供接口清单")
	}

	td := newTableDeps(t)
	td.deps.APIDocs = true
	on := map[string]bool{}
	for _, ri := range Public(td.deps).Routes() {
		on[routeKey(ri.Method, ri.Path)] = true
	}
	if !on[key] {
		t.Error("api_docs.enabled=true 时公开端口没有 /api-docs")
	}
}

// countingRecorder 记录被写入条数的 middleware.OperationRecorder adapter。
type countingRecorder struct{ n int }

func (w *countingRecorder) Record(_ context.Context, _ model.OperationRecord) error {
	w.n++
	return nil
}

// TestPublicTable_OperationLogMountedWhenRecorderGiven 给了写入器就要真的挂上，
// 且只挂在 /api 组上。
//
// Deps.Logs 为 nil 表示「不记审计」，非 nil 时中间件必须在 /api 链上——这个分支
// 此前只出现在生产装配里：路由用例一律传 nil，删掉中间件也不会有人发现。
//
// /health 与 /uploads/avatar 不经过审计中间件：它们不是「某人的一次操作」。
func TestPublicTable_OperationLogMountedWhenRecorderGiven(t *testing.T) {
	td := newTableDeps(t)
	recorder := &countingRecorder{}
	td.deps.Logs = recorder
	engine := Public(td.deps)

	if got := codeOf(t, engine, http.MethodGet, "/api/comments", "", ""); got != "00000" {
		t.Fatalf("/api/comments code = %s, want 00000", got)
	}
	if recorder.n == 0 {
		t.Error("传入了 Logs 却没有写入任何审计：OperationLog 没挂进 /api 路由组")
	}

	before := recorder.n
	if got := codeOf(t, engine, http.MethodGet, "/health", "", ""); got != "00000" {
		t.Fatalf("/health code = %s, want 00000", got)
	}
	if recorder.n != before {
		t.Errorf("/health 不该经过审计中间件，写入增加了 %d 条", recorder.n-before)
	}

	// 静态头像同样不是「某人的一次操作」：它挂在引擎根上，不该被 /api 组中间件捕获
	writeFile(t, filepath.Join(td.deps.AvatarDir, "audit.png"), "png")
	before = recorder.n
	if code, _ := bodyOf(t, engine, "/uploads/avatar/audit.png"); code != http.StatusOK {
		t.Fatalf("头像请求失败：%d", code)
	}
	if recorder.n != before {
		t.Errorf("/uploads/avatar 不该经过审计中间件，写入增加了 %d 条", recorder.n-before)
	}
}

// adminTable 后台表面的每一路（ADR-0010：默认只有本机可达，仍须带后台 token）。
//
// 与公开表同样带 pattern/sample 两列，理由也相同：一路手抄清单挡不住「新加的后台
// 接口没进清单」。/admin/api/login 是唯一不要求 token 的一路——它兑换的就是 token。
var adminTable = []declaredRoute{
	{http.MethodPost, "/admin/api/login", "/admin/api/login", false},
	{http.MethodGet, "/admin", "/admin", false},
	{http.MethodGet, "/admin/", "/admin/", false},
	{http.MethodGet, "/admin/assets/*filepath", "/admin/assets/app.js", false},
	// 后台引擎无条件提供文档：只监听回环，且管理界面要内嵌它。
	{http.MethodGet, "/api-docs/*any", "/api-docs/index.html", false},

	{http.MethodGet, "/admin/api/stats", "/admin/api/stats", true},
	{http.MethodGet, "/admin/api/users", "/admin/api/users", true},
	{http.MethodGet, "/admin/api/users/:uid", "/admin/api/users/1", true},
	{http.MethodGet, "/admin/api/comments", "/admin/api/comments", true},
	{http.MethodDelete, "/admin/api/comments/:id", "/admin/api/comments/1", true},
	{http.MethodPost, "/admin/api/comments/rehash", "/admin/api/comments/rehash", true},
	{http.MethodGet, "/admin/api/logs", "/admin/api/logs", true},
}

func newAdminEngine(td tableDeps) *gin.Engine {
	session := admin.NewSession(config.AdminConfig{
		Username:  "admin",
		Password:  "pass123",
		JWTSecret: "admin-table-secret",
		ExpireMin: 30,
	})
	return AdminConsole(AdminDeps{
		Session: session,
		Handler: admin.NewHandler(td.userRepo, td.comments, td.logRepo, session),
	})
}

// TestAdminConsole_EveryRouteIsDeclared 后台真实路由表与 adminTable 双向相等。
//
// 顺带钉住 ADR-0010 的另一半：公开表面与后台表面各自一张表、各自一个引擎，
// 后台的 API 路由不得出现在公开引擎（上面那条已断过），反之公开 API 也不该
// 被搬到只有回环可达的那个引擎上。
func TestAdminConsole_EveryRouteIsDeclared(t *testing.T) {
	td := newTableDeps(t)
	engine := newAdminEngine(td)

	declared := map[string]bool{}
	for _, r := range adminTable {
		declared[routeKey(r.method, r.pattern)] = true
	}

	actual := map[string]bool{}
	for _, ri := range engine.Routes() {
		key := routeKey(ri.Method, ri.Path)
		actual[key] = true
		if !declared[key] {
			t.Errorf("后台路由 %s 未登记在 adminTable", key)
		}
	}
	for key := range declared {
		if !actual[key] {
			t.Errorf("登记过的后台路由 %s 在真实表里找不到", key)
		}
	}

	// 公开 API 不在这个引擎上：两表面合并等于把 /api/* 一起挪出公网可达范围，
	// 那不是隔离而是把服务关掉，所以这条也得钉住。
	for _, r := range publicTable {
		if strings.HasPrefix(r.pattern, "/api/") {
			if actual[routeKey(r.method, r.pattern)] {
				t.Errorf("公开 API 路由 %s 出现在后台引擎上（两张表必须各自独立）", r.pattern)
			}
		}
	}
}

// TestAdminConsole_EveryRouteRequiresAdminToken 后台每一路都在鉴组里，
// 且用户 token 一概不认（两把密钥互不可用，ADR-0001/0010）。
func TestAdminConsole_EveryRouteRequiresAdminToken(t *testing.T) {
	td := newTableDeps(t)
	engine := newAdminEngine(td)

	userToken, err := td.access.IssueUser(1, "someone")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}

	for _, r := range adminTable {
		if !r.signedIn {
			continue
		}
		t.Run("匿名 "+r.method+" "+r.sample, func(t *testing.T) {
			if got := codeOf(t, engine, r.method, r.sample, "{}", ""); got != "A0403" {
				t.Errorf("code = %s, want A0403（该路漏挂了后台鉴权？）", got)
			}
		})
		t.Run("用户token "+r.method+" "+r.sample, func(t *testing.T) {
			if got := codeOf(t, engine, r.method, r.sample, "{}", userToken); got != "A0403" {
				t.Errorf("code = %s, want A0403（用户 token 不该打得开后台）", got)
			}
		})
	}
}

// TestPublicTable_NoCrossUserLogListing 客户端表面不存在「读全站日志」这一路。
//
// 审计记录每行都带某个人的 IP、UA 与时间线；跨用户的审计读取只在后台
// （/admin/api/logs，ADR-0010 的网络隔离之后）。这里钉住它别以「方便排查」的名义
// 回到公开端口上：/api/logs 必须 404，而 /api/logs/my 必须仍在鉴权组里。
func TestPublicTable_NoCrossUserLogListing(t *testing.T) {
	td := newTableDeps(t)
	engine := Public(td.deps)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/logs status = %d, want 404", w.Code)
	}

	if got := codeOf(t, engine, http.MethodGet, "/api/logs/my", "", ""); got != "A0230" {
		t.Errorf("GET /api/logs/my code = %s, want A0230（该路应挂在鉴权组里）", got)
	}
}

// TestEnginesIgnoreForwardedFor 两个引擎都不得信任客户端提交的代理头。
//
// Gin 默认信任任意来源的 X-Forwarded-For；若照默认走，公开调用者可以把审计里的
// IP 改成任意值（审计失真），也能让后台登录限流按伪造 IP 重新分桶（ADR-0010 的
// 限流被绕过）。这里直接看 c.ClientIP()：它必须取连接的对端地址。
func TestEnginesIgnoreForwardedFor(t *testing.T) {
	td := newTableDeps(t)

	engines := map[string]*gin.Engine{
		"公开": Public(td.deps),
		"后台": newAdminEngine(td),
	}
	for name, engine := range engines {
		t.Run(name, func(t *testing.T) {
			engine.GET("/test-client-ip", func(c *gin.Context) {
				c.String(http.StatusOK, c.ClientIP())
			})

			req := httptest.NewRequest(http.MethodGet, "/test-client-ip", nil)
			req.RemoteAddr = "192.0.2.10:1234"
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			req.Header.Set("X-Real-IP", "203.0.113.9")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			if got := w.Body.String(); got != "192.0.2.10" {
				t.Fatalf("ClientIP() = %q, want 对端地址 192.0.2.10（代理头被信任了）", got)
			}
		})
	}
}

// TestRequestIDWired 两个表面都必须装 RequestID。
//
// 关联 id 由 middleware.RequestID 写入响应头与 gin context，Logger 的 request_id
// 字段靠它串起同一次请求。没有用例时，删掉任一侧的 r.Use(middleware.RequestID())
// 整套测试仍然全绿——排障链路就这样静默断掉。
func TestRequestIDWired(t *testing.T) {
	td := newTableDeps(t)

	engines := map[string]struct {
		engine *gin.Engine
		path   string
	}{
		"公开": {Public(td.deps), "/health"},
		"后台": {newAdminEngine(td), "/admin/api/stats"},
	}
	for name, tc := range engines {
		t.Run(name, func(t *testing.T) {
			// 1) 客户端没给时生成一个，并写进响应头
			w := httptest.NewRecorder()
			tc.engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if got := w.Header().Get(middleware.RequestIDHeader); got == "" {
				t.Fatal("响应缺少 X-Request-ID：RequestID 中间件没有装配")
			}

			// 2) 合法的客户端值必须原样沿用：链路追踪靠它把上游与本次请求串起来
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set(middleware.RequestIDHeader, "trace-abc.123")
			w = httptest.NewRecorder()
			tc.engine.ServeHTTP(w, req)
			if got := w.Header().Get(middleware.RequestIDHeader); got != "trace-abc.123" {
				t.Fatalf("X-Request-ID = %q, want 客户端值 trace-abc.123", got)
			}
		})
	}
}
