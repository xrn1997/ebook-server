package database

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"ebook-server/model"

	"gorm.io/gorm"
)

// 这一组用例是 AutoMigrate 对比用例（TestMigrateSchemaMatchesAutoMigrate）的替代者。
//
// 被删掉的那条把「迁移产物」与「AutoMigrate 产物」逐列比一遍；ADR-0025 让测试库也走
// 迁移之后，两边同源，比较失去了对手。但迁移与模型之间的漂移从此没有任何自动检查：
// 删掉 0001_init.sql 里的一条 UNIQUE、那个 FK 或一条普通索引，全套用例照绿。这里改成
// 直接对内嵌迁移的产物断言性质——约束集就是被测对象，不需要第二个实现来对照。

// tableInfoRow 是 PRAGMA table_info 的一行。
type tableInfoRow struct {
	Cid     int
	Name    string
	Type    string
	Notnull int
	Pk      int
}

// indexListRow 是 PRAGMA index_list 的一行。
type indexListRow struct {
	Seq    int
	Name   string
	Unique int
	Origin string
}

// indexInfoRow 是 PRAGMA index_info 的一行。
type indexInfoRow struct {
	Seqno int
	Cid   int
	Name  string
}

// foreignKeyRow 是 PRAGMA foreign_key_list 的一行。
//
// 只声明本文件要比的列：pragma 的 id 是「同一条外键的多行分组」编号、seq 是组内列序，
// 本仓的外键都是单列，钉它们只会随行序制造假红。外键的身份是 (表, 列→被引用列)。
type foreignKeyRow struct {
	From string
	To   string
	// Table 由 GORM 的列名策略映射到 pragma 的 `table` 列。
	Table string
}

func pragmaTableInfo(t *testing.T, db *gorm.DB, table string) []tableInfoRow {
	t.Helper()
	var rows []tableInfoRow
	if err := db.Raw("PRAGMA table_info(" + table + ")").Scan(&rows).Error; err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	return rows
}

// uniqueIndexesOf 返回该表上所有唯一索引的列组合（排序后），用于跨表比较。
func uniqueIndexesOf(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	var indexes []indexListRow
	if err := db.Raw("PRAGMA index_list(" + table + ")").Scan(&indexes).Error; err != nil {
		t.Fatalf("index_list(%s): %v", table, err)
	}
	var out []string
	for _, idx := range indexes {
		if idx.Unique != 1 {
			continue
		}
		var cols []indexInfoRow
		if err := db.Raw("PRAGMA index_info(" + idx.Name + ")").Scan(&cols).Error; err != nil {
			t.Fatalf("index_info(%s): %v", idx.Name, err)
		}
		names := make([]string, 0, len(cols))
		for _, c := range cols {
			names = append(names, c.Name)
		}
		sort.Strings(names)
		out = append(out, strings.Join(names, "+"))
	}
	sort.Strings(out)
	return out
}

// indexesOf 返回该表上由 `CREATE INDEX`/`CREATE UNIQUE INDEX` 显式建出的索引，
// 形如 `名字(列1+列2)`，按整串排序。
//
// 只收 Origin == "c"：UNIQUE/PRIMARY KEY 的**内联声明**另生 sqlite_autoindex（Origin 为
// "u"/"pk"），那部分性质已由 uniqueIndexesOf 与 primaryKeyOf 钉住。这里钉的是迁移文件里
// 显式写下的那批读路径索引——名字与列序都算。删掉一条、改窄一列、换个名字，都不会让任何
// 功能用例变红：没有测试能观察到一个索引不存在，查询只是悄悄退化成全表扫描。所以这份
// 期望表是它唯一的看守，也是 ADR-0025 里「普通索引」那一面的接管者。
// 列序刻意不排序：多列索引的前缀可用性由顺序决定，换序是真实的行为变化。
func indexesOf(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	var indexes []indexListRow
	if err := db.Raw("PRAGMA index_list(" + table + ")").Scan(&indexes).Error; err != nil {
		t.Fatalf("index_list(%s): %v", table, err)
	}
	var out []string
	for _, idx := range indexes {
		if idx.Origin != "c" {
			continue
		}
		var cols []indexInfoRow
		if err := db.Raw("PRAGMA index_info(" + idx.Name + ")").Scan(&cols).Error; err != nil {
			t.Fatalf("index_info(%s): %v", idx.Name, err)
		}
		names := make([]string, 0, len(cols))
		for _, c := range cols {
			names = append(names, c.Name)
		}
		out = append(out, idx.Name+"("+strings.Join(names, "+")+")")
	}
	sort.Strings(out)
	return out
}

func notNullColumnsOf(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	var out []string
	for _, col := range pragmaTableInfo(t, db, table) {
		if col.Notnull == 1 {
			out = append(out, col.Name)
		}
	}
	sort.Strings(out)
	return out
}

func primaryKeyOf(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	var out []string
	for _, col := range pragmaTableInfo(t, db, table) {
		if col.Pk > 0 {
			out = append(out, col.Name)
		}
	}
	return out
}

// TestMigratedSchemaKeepsItsConstraints 内嵌迁移建出的库必须带着模型依赖的全部约束。
//
// 这份期望表按 0001_init.sql 逐条写出：它不跟着模型走，所以「模型改了、迁移没改」
// 与「迁移被人改窄了」都会在这里变红——后者正是被删掉的对比用例唯一能抓住的东西。
func TestMigratedSchemaKeepsItsConstraints(t *testing.T) {
	db := openRawDB(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cases := []struct {
		table    string
		pk       []string
		unique   []string
		indexes  []string
		notNull  []string
		foreignK []foreignKeyRow
	}{
		{
			table:   "users",
			pk:      []string{"uid"},
			unique:  []string{"email"},
			indexes: []string{"idx_users_deleted_at(deleted_at)", "idx_users_email(email)"},
			notNull: []string{"email", "password", "username"},
		},
		{
			table:   "refresh_tokens",
			pk:      []string{"id"},
			unique:  []string{"token_hash"},
			indexes: []string{"idx_refresh_tokens_token_hash(token_hash)", "idx_refresh_tokens_user_id(user_id)"},
			notNull: []string{"token_hash", "user_id"},
		},
		{
			table:  "comments",
			pk:     []string{"id"},
			unique: nil,
			// comment_key 是公开评论列表的主读路径（`WHERE comment_key IN ?`），chapter_url 与
			// deleted_at 服务兼容查询与软删扫描；user_id 是后台按账号取评论的入口。
			indexes: []string{
				"idx_comments_chapter_url(chapter_url)",
				"idx_comments_comment_key(comment_key)",
				"idx_comments_deleted_at(deleted_at)",
				"idx_comments_user_id(user_id)",
			},
			notNull: []string{"content", "user_id"},
			foreignK: []foreignKeyRow{
				{From: "user_id", To: "uid", Table: "users"},
			},
		},
		{
			table:  "operation_logs",
			pk:     []string{"id"},
			unique: nil,
			// 本仓最大的表：created_at 撑时间序翻页，user_id 撑按账号审计。缺一条就是全表扫描。
			indexes: []string{
				"idx_operation_logs_created_at(created_at)",
				"idx_operation_logs_user_id(user_id)",
			},
			notNull:  nil,
			foreignK: nil,
		},
	}

	for _, tc := range cases {
		if got := primaryKeyOf(t, db, tc.table); !reflect.DeepEqual(got, tc.pk) {
			t.Errorf("%s 主键 = %v, want %v", tc.table, got, tc.pk)
		}
		if got := uniqueIndexesOf(t, db, tc.table); !reflect.DeepEqual(got, tc.unique) {
			t.Errorf("%s 唯一索引列集 = %v, want %v", tc.table, got, tc.unique)
		}
		if got := indexesOf(t, db, tc.table); !reflect.DeepEqual(got, tc.indexes) {
			t.Errorf("%s 显式创建的索引 = %v, want %v", tc.table, got, tc.indexes)
		}
		if got := notNullColumnsOf(t, db, tc.table); !reflect.DeepEqual(got, tc.notNull) {
			t.Errorf("%s NOT NULL 列集 = %v, want %v", tc.table, got, tc.notNull)
		}
		var fks []foreignKeyRow
		if err := db.Raw("PRAGMA foreign_key_list(" + tc.table + ")").Scan(&fks).Error; err != nil {
			t.Fatalf("foreign_key_list(%s): %v", tc.table, err)
		}
		if len(fks) != len(tc.foreignK) {
			t.Fatalf("%s 外键数 = %d, want %d（列/引用表/被引用列见下）: %+v", tc.table, len(fks), len(tc.foreignK), fks)
		}
		for i, want := range tc.foreignK {
			if fks[i].Table != want.Table || fks[i].From != want.From || fks[i].To != want.To {
				t.Errorf("%s 外键 %d = %s(%s→%s), want %s(%s→%s)",
					tc.table, i, fks[i].Table, fks[i].From, fks[i].To, want.Table, want.From, want.To)
			}
		}
	}
}

// TestMigratedUniqueIndexRejectsDuplicateEmail 唯一索引在**库**里，不只在应用层的预检里。
//
// 注册用例走 service 的邮箱存在性预检，索引被删掉也照样绿；这里绕过预检直接写第二行，
// 让「unique 约束没了」这件事在本层就红。
func TestMigratedUniqueIndexRejectsDuplicateEmail(t *testing.T) {
	db := openRawDB(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	seed := &model.User{Email: "dup@example.com", Username: "u1", Nickname: "n", Password: "h"}
	if err := db.Create(seed).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&model.User{Email: "dup@example.com", Username: "u2", Nickname: "n", Password: "h"}).Error; err == nil {
		t.Fatal("idx_users_email 未生效：重复邮箱写进去了")
	}
}

// TestOpenThenMigrateOnFreshFile 首次安装路径：Open(新文件) + Migrate 建表，重开不重放。
//
// 这条组合是生产唯一的启动形状（main.go），而此前 Open 与 Migrate 各自被测、组合没有：
// 第一次装的用户走的正好是没被覆盖的那一条。重开时数据必须在，且版本登记不得增长——
// 重放会把 `CREATE TABLE`/`ALTER` 再跑一遍，前者靠 IF NOT EXISTS 遮掩，后者直接炸。
func TestOpenThenMigrateOnFreshFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate on fresh file: %v", err)
	}
	for _, table := range []string{"users", "comments", "operation_logs", "refresh_tokens", migrateTable} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("missing table %s", table)
		}
	}
	want := embeddedVersions(t)
	if got := appliedVersionsOf(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("applied versions = %v, want %v", got, want)
	}

	user := &model.User{Email: "fresh@example.com", Username: "fresh", Nickname: "n", Password: "h"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := Close(db); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = Close(reopened) })
	if err := Migrate(reopened); err != nil {
		t.Fatalf("Migrate on reopen: %v", err)
	}

	var stored model.User
	if err := reopened.First(&stored, user.UID).Error; err != nil {
		t.Fatalf("数据在重开后丢失: %v", err)
	}
	if got := appliedVersionsOf(t, reopened); !reflect.DeepEqual(got, want) {
		t.Fatalf("重开后版本登记 = %v, want %v（迁移被重放）", got, want)
	}
}
