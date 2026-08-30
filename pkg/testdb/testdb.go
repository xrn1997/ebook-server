// Package testdb 提供测试专用的独立 SQLite :memory: 数据库（ADR-0007）。
//
// 每次调用 Open 都返回一个全新、互不共享状态的库实例，取代历史上
// 「改写全局 database.DB」的做法——那会导致测试无法并行且互相污染。
package testdb

import (
	"testing"

	"ebook-server/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 创建独立的 :memory: 数据库并完成全部实体迁移，连接在测试结束时自动关闭。
func Open(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// SQLite 的 :memory: 库随连接生灭：连接池里的每个新连接都会得到一个空库。
	// 强制单连接，保证同一 *gorm.DB 上的所有操作看到同一个库。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get test database handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(
		&model.User{},
		&model.Comment{},
		&model.OperationLog{},
		&model.RefreshToken{},
	); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
