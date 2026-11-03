package testdb_test

import (
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"testing"

	"ebook-server/migrations"
	"ebook-server/model"
	"ebook-server/pkg/testdb"
)

// TestOpenRunsMigrations 钉住「测试库与生产同走版本化迁移」（ADR-0025）。
//
// Open 建表走 database.Migrate 后，四张业务表齐全且 schema_migrations 里登记的正是
// 内嵌的全部版本；若哪天 Open 退回 AutoMigrate 或别的旁路，这条会当场变红。
// 期望值从 migrations.FS 派生而不是写死当前那一个版本：加第一个合法 0002_*.sql 的
// 人应该看到这条继续通过，而不是被训练成删断言。
func TestOpenRunsMigrations(t *testing.T) {
	db := testdb.Open(t)

	for _, table := range []string{"users", "comments", "operation_logs", "refresh_tokens"} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("missing table %s", table)
		}
	}

	if !db.Migrator().HasTable("schema_migrations") {
		t.Fatal("missing table schema_migrations (test database did not run migrations)")
	}
	var versions []string
	if err := db.Table("schema_migrations").Order("version").Pluck("version", &versions).Error; err != nil {
		t.Fatal(err)
	}
	if want := embeddedVersions(t); !reflect.DeepEqual(versions, want) {
		t.Fatalf("applied versions = %v, want %v", versions, want)
	}
}

// embeddedVersions 列出内嵌迁移 FS 里的全部版本名（按文件名序，与 Migrate 的执行序一致）。
func embeddedVersions(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		versions = append(versions, strings.TrimSuffix(entry.Name(), ".sql"))
	}
	sort.Strings(versions)
	if len(versions) == 0 {
		t.Fatal("内嵌迁移 FS 里没有任何 .sql，这条用例本身就失去意义")
	}
	return versions
}

// TestOpenEnforcesForeignKeys 测试库与生产一样开启外键约束（ADR-0025）。
//
// 「同一条迁移路径」只保证 DDL 一致；外键是**按连接**生效的 pragma，没开时库里照样
// 有 fk_comments_user 这条定义，但插入孤儿评论不会报错。那样的测试库比生产宽松，
// 违反约束的用例一路绿到部署机才炸——所以这里钉的是约束真的在生效，不是它存在。
func TestOpenEnforcesForeignKeys(t *testing.T) {
	db := testdb.Open(t)

	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1（测试库跑在比生产宽松的库里）", foreignKeys)
	}

	// 行为断言：user_id 指向不存在的账号，评论必须写不进去。
	err := db.Create(&model.Comment{UserID: 4242, Content: "orphan", CommentKey: "ck:orphan"}).Error
	if err == nil {
		t.Fatal("外键未生效：引用不存在账号的评论写进去了")
	}

	// 对照：同一行在有账号引用时能写进去，排除「表根本写不了」这种假通过。
	if err := db.Create(&model.User{UID: 7, Email: "fk@example.com", Username: "fk", Password: "$2a$10$x"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&model.Comment{UserID: 7, Content: "ok", CommentKey: "ck:ok"}).Error; err != nil {
		t.Fatalf("外键不该拦住有归属的评论: %v", err)
	}
}
