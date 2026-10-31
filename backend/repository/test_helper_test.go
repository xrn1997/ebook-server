package repository

import (
	"context"
	"testing"

	"ebook-server/model"
	"ebook-server/pkg/testdb"

	"gorm.io/gorm"
)

// testCtx 测试直接调用 repository 时使用的请求上下文。
var testCtx = context.Background()

// testDB 当前测试的独立 :memory: 数据库。
//
// 每次 setupTestDB 都创建全新实例，测试之间互不共享状态；
// go test 同包默认串行执行，包内变量在此是安全的（ADR-0007）。
var testDB *gorm.DB

// setupTestDB 创建本测试专用的 :memory: 数据库（连接由 testdb 在测试结束时关闭）
func setupTestDB(t *testing.T) {
	t.Helper()
	testDB = testdb.Open(t)
}

// cleanupTestDB 释放当前测试的数据库引用（连接关闭由 testdb 的 t.Cleanup 负责）
func cleanupTestDB(t *testing.T) {
	t.Helper()
	testDB = nil
}

// seedUser 造一个账号，返回其 UID。
func seedUser(t *testing.T, email, username, nickname string) uint {
	t.Helper()
	user := &model.User{Email: email, Password: "hp", Username: username, Nickname: nickname}
	if err := NewUserRepository(testDB).Create(testCtx, user); err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
	return user.UID
}
