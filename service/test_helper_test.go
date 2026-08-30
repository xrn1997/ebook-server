package service

import (
	"testing"

	"ebook-server/config"
	"ebook-server/pkg/code"
	"ebook-server/pkg/mail"
	"ebook-server/pkg/testdb"
	"ebook-server/repository"

	"gorm.io/gorm"
)

// 测试环境：一套相互独立的依赖（独立 :memory: 库、独立验证码存储与服务实例）。
//
// 每次 setupTestDB 全部重建，测试之间互不共享状态；go test 同包默认串行执行，
// 包内变量在此是安全的（ADR-0007）。
var (
	testDB       *gorm.DB
	testCodes    *code.Store
	testAuth     *AuthService
	testUsers    *UserService
	testAccount  *AccountService
	testComments *CommentService
	testLogs     *LogService
)

// setupTestDB 组装本测试专用的依赖环境
func setupTestDB(t *testing.T) {
	t.Helper()

	// jwt 签发读取 config.AppConfig.JWT（历史耦合），测试需提供密钥
	config.AppConfig = &config.Config{
		Server: config.ServerConfig{Mode: "test"},
		JWT: config.JWTConfig{
			Secret:    "test-secret",
			ExpireMin: 1,
		},
	}

	testDB = testdb.Open(t)

	users := repository.NewUserRepository(testDB)
	tokens := repository.NewRefreshTokenRepository(testDB)
	comments := repository.NewCommentRepository(testDB)
	logs := repository.NewLogRepository(testDB)
	testCodes = code.NewStore()

	// 测试注入写日志的 Mailer：验证码经 testCodes 直接注入，不走真实邮件
	testAuth = NewAuthService(users, tokens, testCodes, mail.NewLogMailer())
	testUsers = NewUserService(users)
	testAccount = NewAccountService(users, tokens, comments, testCodes, mail.NewLogMailer())
	testComments = NewCommentService(comments)
	testLogs = NewLogService(logs)
}

// cleanupTestDB 释放当前测试的环境引用（连接关闭由 testdb 的 t.Cleanup 负责）
func cleanupTestDB(t *testing.T) {
	t.Helper()
	testDB = nil
	testCodes = nil
	testAuth = nil
	testUsers = nil
	testAccount = nil
	testComments = nil
	testLogs = nil
}
