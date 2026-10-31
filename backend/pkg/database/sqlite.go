// Package database 提供 SQLite + GORM 的连接生命周期管理。
package database

import (
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DefaultPath 未配置数据库路径时使用的文件名。
const DefaultPath = "ebook.db"

// busyTimeoutMS SQLite 写锁等待上限；WAL + busy_timeout 让并发写等待而不是直接报锁。
const busyTimeoutMS = 5000

// Open 打开 SQLite 数据库并配置连接池。
//
// 路径为空时退回 DefaultPath。返回连接而不是写全局：调用方（装配点）持有它，
// 于是「必须先 Init 再 GetDB」这条隐式顺序不再是每个使用方的前提。
//
// busy_timeout 与 foreign_keys 是**按连接**生效的 pragma，不能只在一条连接上执行；
// 因此把它们放进 DSN 的 _pragma 参数，由驱动在每条新连接上执行。journal_mode=WAL
// 是库级设置，但同样放这里，保证首次打开就生效。
func Open(path string) (*gorm.DB, error) {
	if path == "" {
		path = DefaultPath
	}

	db, err := gorm.Open(sqlite.Open(dsnOf(path)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// 连接池配置
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(10)

	zap.L().Info("Database connected successfully", zap.String("path", path))
	return db, nil
}

// dsnOf 给文件库附加 SQLite pragma；:memory: 由测试库自己管理连接，不附加。
func dsnOf(path string) string {
	if path == ":memory:" {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%s_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path, sep, busyTimeoutMS)
}

// Close 关闭底层连接池，释放 SQLite 文件句柄。
//
// 供优雅退出调用：连接池里的空闲连接不会自己断开，句柄不释放时 Windows 上数据库
// 文件持续被占用，紧接着的备份或替换会失败。
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	return sqlDB.Close()
}
