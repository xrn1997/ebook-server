// Package database 提供数据库连接初始化与全局访问（SQLite + GORM）。
package database

import (
	"ebook-server/config"
	"fmt"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 全局数据库连接实例，由 Init 初始化。
var DB *gorm.DB

// Init 初始化 SQLite 数据库连接并配置连接池。
func Init() error {
	dbPath := config.AppConfig.Database.Path
	if dbPath == "" {
		dbPath = "ebook.db"
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// 连接池配置
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(10)

	DB = db
	zap.L().Info("Database connected successfully", zap.String("path", dbPath))
	return nil
}

// GetDB 返回全局数据库连接实例。
func GetDB() *gorm.DB {
	return DB
}

// Close 关闭底层连接池，释放 SQLite 文件句柄。
//
// 供优雅退出调用：连接池里的空闲连接不会自己断开，句柄不释放时 Windows 上数据库
// 文件持续被占用，紧接着的备份或替换会失败。
func Close() error {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	return sqlDB.Close()
}
