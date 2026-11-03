package database

import (
	"path/filepath"
	"strings"
	"testing"

	"ebook-server/model"
)

func TestOpenAppliesSQLitePragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pragmas.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })

	var journalMode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if busyTimeout != busyTimeoutMS {
		t.Fatalf("busy_timeout = %d, want %d", busyTimeout, busyTimeoutMS)
	}

	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
}

// TestDSNOfCarriesConstraintsOnBothShapes 按连接生效的 pragma 在两类库上都要附加。
//
// 内存库曾经被原样返回（「由测试库自己管理」），于是所有走 :memory: 的用例都在
// 外键关闭的库里跑——比生产宽松，违反约束的写入只有到部署机才报错。
// journal_mode=WAL 是文件库独有的：内存库没有 WAL 可言，不附加。
func TestDSNOfCarriesConstraintsOnBothShapes(t *testing.T) {
	memory := dsnOf(":memory:")
	if !strings.Contains(memory, "_pragma=foreign_keys(1)") || !strings.Contains(memory, "_pragma=busy_timeout") {
		t.Fatalf("memory DSN 必须带 busy_timeout 与 foreign_keys, got %q", memory)
	}
	if strings.Contains(memory, "journal_mode") {
		t.Fatalf("内存库不应附加 WAL, got %q", memory)
	}

	file := dsnOf("ebook.db")
	if !strings.Contains(file, "_pragma=foreign_keys(1)") || !strings.Contains(file, "_pragma=journal_mode(WAL)") {
		t.Fatalf("file DSN 应带全部 pragma, got %q", file)
	}
}

// TestOpenMemoryEnforcesForeignKeys 行为层面确认 DSN 里的设置真的落到了连接上。
func TestOpenMemoryEnforcesForeignKeys(t *testing.T) {
	db, err := OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	// comments.user_id 的外键指向 users.uid；没有引用行时写入必须失败。
	if err := db.Create(&model.Comment{UserID: 999, Content: "orphan", CommentKey: "ck:orphan"}).Error; err == nil {
		t.Fatal("外键未生效：孤儿评论写进去了")
	}
}

func TestDSNOfPreservesExistingQuery(t *testing.T) {
	got := dsnOf("file.db?cache=shared")
	if !strings.Contains(got, "&_pragma=busy_timeout") {
		t.Fatalf("existing query must be extended with &, got %q", got)
	}
}

func TestCloseNilIsNoop(t *testing.T) {
	if err := Close(nil); err != nil {
		t.Fatalf("Close(nil) = %v, want nil", err)
	}
}
