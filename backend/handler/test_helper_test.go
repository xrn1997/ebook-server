package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ebook-server/config"
	"ebook-server/middleware"
	"ebook-server/pkg/code"
	"ebook-server/pkg/mail"
	"ebook-server/pkg/testdb"
	"ebook-server/pkg/upload"
	"ebook-server/repository"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// testCode 测试用注册验证码
const testCode = "123456"

// testConfig 测试配置
var testConfig = &config.Config{
	Server: config.ServerConfig{
		Port: 8080,
		Mode: "test",
	},
	Database: config.DatabaseConfig{
		Path: ":memory:",
	},
	JWT: config.JWTConfig{
		Secret:    "test-secret-key",
		ExpireMin: 1,
	},
}

// testCodes 最近一次 newTestApp 创建的验证码存储。
//
// go test 同包默认串行执行，registerUser 等助手经它注入验证码（ADR-0007）。
var testCodes *code.Store

// testApp 一套相互独立的测试应用：独立 :memory: 库、验证码存储与服务实例。
//
// 每个测试通过 newTestApp 获得全新环境，替代历史上「改写全局 database.DB +
// 共享 code.Default()」的做法——那会导致测试互相污染且无法并行。
type testApp struct {
	db      *gorm.DB
	codes   *code.Store
	auth    *AuthHandler
	user    *UserHandler
	account *AccountHandler
	comment *CommentHandler
	log     *LogHandler
	uploads *UploadHandler
}

// newTestApp 组装本测试专用的处理器集合。
//
// 所有 handler 共享同一套依赖，与 main.go 的装配方式一致。
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	config.AppConfig = testConfig

	db := testdb.Open(t)
	users := repository.NewUserRepository(db)
	tokens := repository.NewRefreshTokenRepository(db)
	comments := repository.NewCommentRepository(db)
	logs := repository.NewLogRepository(db)
	codes := code.NewStore()
	testCodes = codes

	// 测试注入写日志的 Mailer：验证码经 codes 直接注入，不走真实邮件
	sender := service.NewVerificationCodeSender(codes, mail.NewLogMailer())
	authSvc := service.NewAuthService(users, tokens, codes, sender)
	accountSvc := service.NewAccountService(users, tokens, comments, codes, sender)
	uploads := upload.New(t.TempDir())

	return &testApp{
		db:      db,
		codes:   codes,
		auth:    NewAuthHandler(authSvc),
		user:    NewUserHandler(service.NewUserService(users, uploads), authSvc),
		account: NewAccountHandler(accountSvc),
		comment: NewCommentHandler(service.NewCommentService(comments)),
		log:     NewLogHandler(service.NewLogService(logs)),
		uploads: NewUploadHandler(uploads),
	}
}

// doJSON 发一个 JSON 请求并返回响应记录器。
//
// 「marshal → NewRequest → 设 Content-Type/Authorization → ServeHTTP」这四步原本
// 在每个评论用例里抄一遍（AGENTS.md：handler 公共工具放本文件）。
// body 传 nil 表示无请求体（GET/DELETE）；token 传空串表示匿名请求。
func doJSON(t *testing.T, router *gin.Engine, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body failed: %v", err)
		}
		reader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, path, reader)
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// commentRoutes 注册评论用例需要的路由：认证两端 + 创建/列表/聚合键迁移。
//
// 需要其中一部分的测试也用它（多注册的路由不调用即无副作用），省下五行装配样板。
func commentRoutes(t *testing.T, router *gin.Engine, app *testApp) {
	t.Helper()
	router.POST("/api/auth/register", app.auth.Register)
	router.POST("/api/auth/login", app.auth.Login)
	router.POST("/api/comments", middleware.JWTAuth(), app.comment.Create)
	router.GET("/api/comments", app.comment.GetList)
	router.POST("/api/comments/migrate", middleware.JWTAuth(), app.comment.MigrateKey)
}

// createComment 以 token 身份发一条归属 commentKey 聚合键的评论，失败即终止用例。
//
// 键走 M2 契约的 comment_key 字段（创建评论必填）：不带键的创建会被服务端当场拒绝。
// 需要造 chapter_url 时代的旧行时用 seedLegacyComment——换键后那条写路径已不可达。
// 刻意断言成功：造数据阶段的失败不是被测行为，留在这里失败比让后续断言
// 报出迷惑性错误更好定位。
func createComment(t *testing.T, router *gin.Engine, token, content, commentKey string) {
	t.Helper()
	w := doJSON(t, router, http.MethodPost, "/api/comments", map[string]string{
		"content": content, "comment_key": commentKey,
	}, token)
	assertErrorCode(t, w.Body.Bytes(), "00000")
}

// decodeData 解析响应信封并返回 data（成功时 code 为 00000）
func decodeData(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if resp["code"] != "00000" {
		t.Fatalf("Expected success code, got %v (error=%v)", resp["code"], resp["error"])
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be an object")
	}
	return data
}

// registerUser 完整走一遍「注入注册码 → 注册 → 登录」，返回 (uid, accessToken)
func registerUser(t *testing.T, router *gin.Engine, email string) (uint, string) {
	t.Helper()
	return registerUserWith(t, router, email, "password123")
}

// registerUserWith 指定密码的注册+登录助手
func registerUserWith(t *testing.T, router *gin.Engine, email, password string) (uint, string) {
	t.Helper()
	// 直接注入注册码，绕过邮件链路（注册码 key 为 reg:<email>）
	testCodes.Set("reg:"+email, testCode)

	body := map[string]string{
		"email":    email,
		"code":     testCode,
		"password": password,
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	// 注册成功返回值不带 token，仅校验业务码
	assertErrorCode(t, w.Body.Bytes(), "00000")

	// 登录取 token 与 uid
	loginBody := map[string]string{"email": email, "password": password}
	jsonBody, _ = json.Marshal(loginBody)
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	data := decodeData(t, w.Body.Bytes())
	user := data["user"].(map[string]interface{})
	return uint(user["uid"].(float64)), data["token"].(string)
}

// assertErrorCode 断言响应信封中的业务错误码
func assertErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if resp["code"] != want {
		t.Errorf("Expected code %s, got %v (error=%v)", want, resp["code"], resp["error"])
	}
}

// assertErrorMessage 断言响应信封中的业务错误文案
//
// 防账号枚举场景必须与 assertErrorCode 配套使用：业务码相同而文案不同，
// 攻击者仍可凭 error 字段区分邮箱是否已注册（ADR-0006）。
func assertErrorMessage(t *testing.T, body []byte, want string) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if resp["error"] != want {
		t.Errorf("Expected error %q, got %v (code=%v)", want, resp["error"], resp["code"])
	}
}
