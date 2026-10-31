package service

import (
	"context"
	"testing"

	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/jwt"
	"ebook-server/pkg/testdb"
	"ebook-server/pkg/upload"
	"ebook-server/repository"

	"gorm.io/gorm"
)

// testCtx 测试直接调用 service 时使用的请求上下文。
var testCtx = context.Background()

// 测试环境：一套相互独立的依赖（独立 :memory: 库、独立验证码存储与服务实例）。
//
// 每次 setupTestDB 全部重建，测试之间互不共享**业务**状态。但下面这些本身是包级
// 变量：go test 同包默认串行执行，所以它们安全，代价是**这里不能加 t.Parallel()**
// ——两个用例交错跑起来，cleanupTestDB 会把对方正在用的句柄置空。
//
// （ADR-0007 消掉的是**生产代码**的包级全局：配置、JWT 密钥、数据库连接都以值注入，
// 所以组装这套环境不需要先有人加载过配置。测试夹具本身仍是全局，handler 包同理。）
var (
	testDB       *gorm.DB
	testCodes    *code.Store
	testMails    *recordingMailer // 被测服务的 Mailer adapter：断言「有没有发、发到哪」
	testTokens   *jwt.TokenBox    // 用户 access token 单元
	testAuth     *AuthService
	testUsers    *ProfileService
	testAccount  *AccountService
	testComments *CommentService
	testLogs     *LogService
)

// setupTestDB 组装本测试专用的依赖环境
func setupTestDB(t *testing.T) {
	t.Helper()

	testDB = testdb.Open(t)

	users := repository.NewUserRepository(testDB)
	tokens := repository.NewRefreshTokenRepository(testDB)
	comments := repository.NewCommentRepository(testDB)
	logs := repository.NewLogRepository(testDB)
	testCodes = code.NewStore()
	testMails = &recordingMailer{}
	testTokens = jwt.New("test-secret", 1)

	// 验证码模块用记录型 Mailer：既能经真实下发链路预置码，又能断言防枚举的静默
	sender := NewVerificationCodes(testCodes, testMails)
	credentials := NewCredentials(users, tokens)
	testAuth = NewAuthService(users, tokens, sender, testTokens, credentials)
	testUsers = NewProfileService(users, upload.New(t.TempDir(), ""))
	testAccount = NewAccountService(users, tokens, comments, sender, testUsers)
	testComments = NewCommentService(comments)
	testLogs = NewLogService(logs)
}

// seedCode 经验证码模块真实下发一枚码并返回它。
//
// 预置码只有 Send 这一条路：pkg/code 不提供「写入指定码」的入口，测试因此
// 不需要（也无法）猜存储键的命名空间格式。
// 用独立实例下发——它自带一对限流窗口，所以既不占用被测服务 SendCode 的配额，
// 写进的码又与它共享同一个 code.Store。
func seedCode(t *testing.T, flow Flow, email string) string {
	t.Helper()

	rec := &recordingMailer{}
	if err := NewVerificationCodes(testCodes, rec).Send(testCtx, flow, email); err != nil {
		t.Fatalf("预置 %s 流程验证码失败: %v", flow, err)
	}
	codeVal := rec.last(email)
	if codeVal == "" {
		t.Fatalf("预置验证码未派发到 %s", email)
	}
	return codeVal
}

// seedCodeWrong 预置一枚码到指定流程，并返回必然不等于它的错码。
func seedCodeWrong(t *testing.T, flow Flow, email string) string {
	t.Helper()
	if real := seedCode(t, flow, email); real != "000000" {
		return "000000"
	}
	return "000001"
}

// cleanupTestDB 释放当前测试的环境引用（连接关闭由 testdb 的 t.Cleanup 负责）
func cleanupTestDB(t *testing.T) {
	t.Helper()

	testDB = nil
	testCodes = nil
	testMails = nil
	testAuth = nil
	testUsers = nil
	testAccount = nil
	testComments = nil
	testLogs = nil
}

// seedLegacyComment 直接经数据层写入一条换轨前的历史行（空 comment_key）。
//
// 空键是「迁移尚未发生」的既成事实，只能由数据层造出来：CommentService.Create 现在
// 拒绝空键（那会成为任何 comment_keys 查询都命中不到的黑洞，ADR-0014），测试不能
// 借创建路径给历史数据开后门。
func seedLegacyComment(t *testing.T, userID uint, content, chapterURL, bookName string) {
	t.Helper()

	if err := repository.NewCommentRepository(testDB).Create(testCtx, &model.Comment{
		UserID:     userID,
		Content:    content,
		ChapterURL: chapterURL,
		BookName:   bookName,
	}); err != nil {
		t.Fatalf("seed legacy comment: %v", err)
	}
}
