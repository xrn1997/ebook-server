package database

import (
	"path/filepath"
	"strings"
	"testing"
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

func TestDSNOfMemoryIsUnchanged(t *testing.T) {
	if got := dsnOf(":memory:"); got != ":memory:" {
		t.Fatalf("dsnOf(:memory:) = %q", got)
	}
	if got := dsnOf("ebook.db"); got == "ebook.db" {
		t.Fatal("file DSN should carry pragmas")
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
