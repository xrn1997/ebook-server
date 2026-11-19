// Package database 提供 SQLite + GORM 的连接生命周期管理。
package database

import (
	"fmt"
	"os"
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
// 路径为空时退回 DefaultPath。返回连接而不是写全局：调用方（装配点）持有它。
//
// pragma 的取舍在 dsnOf：按连接生效，只能放 DSN。
//
// 默认装配非 debug 档（参数占位、只记慢查询与错误）：调用方不会因为忘记配置
// logger 把 SQL 参数写进生产日志；装配点读到运行模式后可用 NewLogger 覆盖。
func Open(path string) (*gorm.DB, error) {
	if path == "" {
		path = DefaultPath
	}

	db, err := gorm.Open(sqlite.Open(dsnOf(path)), &gorm.Config{
		Logger: NewLogger(false, os.Stdout),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(10)

	zap.L().Info("Database connected successfully", zap.String("path", path))
	return db, nil
}

// dsnOf 给连接附加 SQLite pragma。busy_timeout 与 foreign_keys 两类库都要附加：
// 按连接生效的设置，测试库与生产库必须看到同一套约束（ADR-0025）。
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
// :memory: 库随连接生灭，池里每条新连接都会得到一个空库，单连接是正确性前提
// 而不是优化。日志静音：调用方是测试，SQL 报错刷屏会埋掉真正的断言失败。
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
// 供优雅退出调用：空闲连接不会自己断开，句柄不释放时 Windows 上数据库文件持续
// 被占用，紧接着的备份或替换会失败。
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
