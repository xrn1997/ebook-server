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
// pragma 的取舍在 dsnOf：按连接生效，所以只能放 DSN，不能只在某一条连接上执行。
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

// dsnOf 给连接附加 SQLite pragma。busy_timeout 与 foreign_keys 两类库都要附加：
// 它们是按连接生效的设置，测试库与生产库必须看到同一套约束（ADR-0025）——
// 只在文件库上开外键，等于所有走 :memory: 的用例都在没有约束的库里跑。
// journal_mode=WAL 只对文件库有意义，内存库不附加。
func dsnOf(path string) string {
	constraints := fmt.Sprintf("_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)", busyTimeoutMS)
	if path == ":memory:" {
		return path + "?" + constraints
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%s%s&_pragma=journal_mode(WAL)", path, sep, constraints)
}

// OpenMemory 打开一个内存库：pragma 与文件库同源，连接池锁成单连接。
//
// :memory: 库随连接生灭，池里的每一条新连接都会得到一个空库，所以这里不是
// 「优化」而是正确性前提。日志静音：调用方是测试，一条 SQL 报错刷屏会把
// 真正的断言失败埋掉。
func OpenMemory() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsnOf(":memory:")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect in-memory database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db, nil
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
