package repository

import (
	"testing"

	"ebook-server/pkg/testdb"

	"gorm.io/gorm"
)

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
