// Package testdb 提供测试专用的独立 SQLite :memory: 数据库。
//
// 每次调用 Open 都返回一个全新、互不共享状态的库实例，测试可并行且互不污染
// （ADR-0007：不改写全局 database.DB）。
package testdb

import (
	"testing"

	"ebook-server/pkg/database"

	"gorm.io/gorm"
)

// Open 创建独立的 :memory: 数据库并完成建表，连接在测试结束时自动关闭。
//
// 连接与 pragma 由 database.OpenMemory 给出（外键约束、写锁等待与生产同源）；
// 建表走 database.Migrate——与生产同一条迁移路径，建表事实源唯一。
func Open(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.OpenMemory()
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	t.Cleanup(func() { _ = database.Close(db) })
	return db
}
