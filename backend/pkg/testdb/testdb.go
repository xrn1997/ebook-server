// Package testdb 提供测试专用的独立 SQLite :memory: 数据库（ADR-0007）。
//
// 每次调用 Open 都返回一个全新、互不共享状态的库实例，取代历史上
// 「改写全局 database.DB」的做法——那会导致测试无法并行且互相污染。
package testdb

import (
	"testing"

	"ebook-server/pkg/database"

	"gorm.io/gorm"
)

// Open 创建独立的 :memory: 数据库并完成建表，连接在测试结束时自动关闭。
//
// 连接与 pragma 都由 database.OpenMemory 给出：外键约束与写锁等待和生产同源，
// 测试不再跑在一个「约束没开」的库里。建表走 database.Migrate——与生产同一条
// 迁移路径，测试库与生产库不再有两套建表事实源（模型清单也自持在 Migrate 内部，
// 这里同样不传清单）。
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
