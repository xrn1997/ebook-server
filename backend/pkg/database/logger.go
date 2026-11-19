package database

import (
	"io"
	"log"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

// gormSlowQuery 慢查询阈值：超过即按 Warn 记账。
const gormSlowQuery = 200 * time.Millisecond

// NewLogger 按运行模式装配 GORM 的 SQL logger。
//
// debug：Info 档，全量 SQL + 内联参数——联调要看得到实际取值。注意
// gormlogger.Default 自身是 Warn 档，不能直接拿它当「框架默认」返回：那会把
// debug 的 SQL 日志静默关掉。
//
// 其余模式只留 Warn 及以上（慢查询与语句错误），且有三处必须显式设定，
// 单靠 LogMode(Warn) 都挡不住：
//   - ParameterizedQueries：GORM 的 Trace 在错误/慢查询分支会把参数内联进
//     SQL 文本——未注册邮箱的登录、重复注册失败会把明文邮箱写进日志，
//     令牌哈希、评论内容同理。置 true 后日志里只剩 `?` 占位。
//   - IgnoreRecordNotFoundError：ErrRecordNotFound 是登录/查询的常规控制流，
//     Warn 级别（Warn ≥ Error）会把每次「查无此人」连 SQL 一起记成错误。
//   - Colorful:false：生产输出进文件/journald，ANSI 色码只会污染日志。
//
// 落库审计仍由 operation_logs 承担；输出流是参数，测试注入内存 buffer。
func NewLogger(debug bool, out io.Writer) gormlogger.Interface {
	writer := log.New(out, "\r\n", log.LstdFlags)
	if debug {
		return gormlogger.New(writer, gormlogger.Config{
			SlowThreshold: gormSlowQuery,
			LogLevel:      gormlogger.Info,
			Colorful:      true,
		})
	}
	return gormlogger.New(writer, gormlogger.Config{
		SlowThreshold:             gormSlowQuery,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      true,
		Colorful:                  false,
	})
}
