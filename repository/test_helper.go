package repository

import (
	"ebook-server/config"
	"ebook-server/model"
	"ebook-server/pkg/database"
	"testing"
)

// setupTestDB 初始化 SQLite :memory: 数据库并自动迁移
func setupTestDB(t *testing.T) {
	t.Helper()

	// 重置数据库连接
	if database.DB != nil {
		sqlDB, _ := database.DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		database.DB = nil
	}

	config.AppConfig = &config.Config{
		Server: config.ServerConfig{Mode: "test"},
		JWT: config.JWTConfig{
			Secret:    "test-secret",
			ExpireMin: 1,
		},
		Database: config.DatabaseConfig{
			Path: ":memory:",
		},
	}

	if err := database.Init(); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}

	database.GetDB().AutoMigrate(
		&model.User{},
		&model.Comment{},
		&model.OperationLog{},
		&model.RefreshToken{},
	)
}

// cleanupTestDB 关闭数据库连接
func cleanupTestDB(t *testing.T) {
	t.Helper()
	if database.DB != nil {
		sqlDB, _ := database.DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		database.DB = nil
	}
}
