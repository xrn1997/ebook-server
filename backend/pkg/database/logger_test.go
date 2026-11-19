package database

import (
	"bytes"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func openLoggerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	return db
}

func createUsersTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("CREATE TABLE users (email TEXT)").Error; err != nil {
		t.Fatalf("创建 users 表: %v", err)
	}
}

// TestNewLogger_NonDebugHidesParams 非 debug 模式下，出错 SQL 不得把参数值写进日志。
//
// 这是生产日志隐私契约的回归锚（契约本身见 NewLogger 的注释）：LogMode(Warn)
// 单改级别挡不住参数内联——GORM 在错误分支仍会 ExplainSQL 把 `?` 还原成明文。
// 未注册邮箱登录、重复注册都会走到这里，若参数进了日志，journald 里就躺着邮箱/令牌哈希。
func TestNewLogger_NonDebugHidesParams(t *testing.T) {
	const secret = "leak-check@example.com"

	db := openLoggerTestDB(t)
	var buf bytes.Buffer
	db.Logger = NewLogger(false, &buf)

	// 往不存在的表写带参数的 SQL：必走 Trace 的 error 分支，正是参数泄露的通道。
	if err := db.Table("no_such_table").Create(map[string]any{"email": secret}).Error; err == nil {
		t.Fatal("预期 SQL 出错以触发错误日志，却没有")
	}

	out := buf.String()
	if out == "" {
		t.Fatal("非 debug 下 SQL 错误应留下 Warn 日志，结果日志为空（可能连错误都吞了）")
	}
	if strings.Contains(out, secret) {
		t.Fatalf("日志泄露了 SQL 参数值 %q:\n%s", secret, out)
	}
}

// TestNewLogger_NonDebugSilencesRecordNotFound ErrRecordNotFound 是常规控制流，
// 不该在 Warn 下刷错误日志（否则「查无此人」的登录尝试会连 SQL 一起进 journald）。
//
// 断言看的是日志本身，不是参数值：参数被占位化后「邮箱没出现」并不能证明这条错误
// 没被记下来，只有「record not found」缺席才说明 IgnoreRecordNotFoundError 生效。
func TestNewLogger_NonDebugSilencesRecordNotFound(t *testing.T) {
	db := openLoggerTestDB(t)
	createUsersTable(t, db)
	var buf bytes.Buffer
	db.Logger = NewLogger(false, &buf)

	var row map[string]any
	err := db.Table("users").Where("email = ?", "nobody@example.com").Take(&row).Error
	if err != gorm.ErrRecordNotFound {
		t.Fatalf("want ErrRecordNotFound, got %v", err)
	}
	if out := strings.ToLower(buf.String()); strings.Contains(out, "record not found") {
		t.Fatalf("RecordNotFound 被记进日志:\n%s", buf.String())
	}
}

// TestNewLogger_DebugLogsSQLWithParams debug 模式必须保留全量 SQL 与内联参数。
//
// database.Open 的默认档位是非 debug（参数占位、只记慢查询与错误）；联调要靠
// debug 档看到实际取值。这条注入内存 buffer，钉住「SQL 真的被输出」且「参数没有
// 被占位成 ?」。
func TestNewLogger_DebugLogsSQLWithParams(t *testing.T) {
	db := openLoggerTestDB(t)
	createUsersTable(t, db)
	var buf bytes.Buffer
	db.Logger = NewLogger(true, &buf)

	var rows []map[string]any
	db.Table("users").Where("email = ?", "debug@example.com").Find(&rows)

	out := buf.String()
	if !strings.Contains(out, "SELECT") {
		t.Fatalf("debug 模式没有输出 SQL（联调看不到语句）:\n%s", out)
	}
	if !strings.Contains(out, "debug@example.com") {
		t.Fatalf("debug 模式 SQL 未内联参数（联调看不到实际取值）:\n%s", out)
	}
}
