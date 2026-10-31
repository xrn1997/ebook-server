package database

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"ebook-server/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testModels() []interface{} {
	return []interface{}{&model.User{}, &model.Comment{}, &model.OperationLog{}, &model.RefreshToken{}}
}

func openRawDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestMigrateFreshDatabase(t *testing.T) {
	db := openRawDB(t)
	if err := Migrate(db, testModels()...); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, table := range []string{"users", "comments", "operation_logs", "refresh_tokens", migrateTable} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("missing table %s", table)
		}
	}

	var count int64
	if err := db.Table(migrateTable).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("schema_migrations rows = %d, want 1", count)
	}

	if err := Migrate(db, testModels()...); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if err := db.Table(migrateTable).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("schema_migrations rows after second run = %d, want 1", count)
	}
}

func TestMigrateAdoptsLegacyDatabase(t *testing.T) {
	db := openRawDB(t)
	if err := db.AutoMigrate(testModels()...); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db, testModels()...); err != nil {
		t.Fatalf("Migrate legacy: %v", err)
	}

	var version string
	if err := db.Table(migrateTable).Select("version").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != "0001_init" {
		t.Fatalf("version = %q, want 0001_init", version)
	}
}

// TestMigrateAdoptionMarksEveryEmbeddedVersion 收养必须把当前二进制的全部版本记为已应用。
//
// AutoMigrate 已经把库带到当前 models 的最新 schema；若只标记 0001，未来迁移会在
// 下一次启动时重放。这条用例里的 0002 正是一个非幂等 ALTER：一旦重放就因列已存在
// 而失败，而失败发生在「迁移收养本应保护的升级路径」上。
func TestMigrateAdoptionMarksEveryEmbeddedVersion(t *testing.T) {
	db := openRawDB(t)
	if err := db.AutoMigrate(testModels()...); err != nil {
		t.Fatal(err)
	}

	fsys := fstest.MapFS{
		"0001_init.sql":         &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS baseline_marker (id integer);")},
		"0002_add_nickname.sql": &fstest.MapFile{Data: []byte("ALTER TABLE users ADD COLUMN nickname text;")},
	}
	if err := migrateFS(db, fsys, testModels()...); err != nil {
		t.Fatalf("legacy adoption replayed a future migration: %v", err)
	}

	var versions []string
	if err := db.Table(migrateTable).Order("version").Pluck("version", &versions).Error; err != nil {
		t.Fatal(err)
	}
	want := []string{"0001_init", "0002_add_nickname"}
	if len(versions) != len(want) || versions[0] != want[0] || versions[1] != want[1] {
		t.Fatalf("applied versions = %v, want %v", versions, want)
	}
}

// TestMigrateFreshDatabaseRunsEveryMigration 新库仍按顺序执行全部迁移。
func TestMigrateFreshDatabaseRunsEveryMigration(t *testing.T) {
	db := openRawDB(t)
	fsys := fstest.MapFS{
		"0001_init.sql":     &fstest.MapFile{Data: []byte("CREATE TABLE widgets (id integer PRIMARY KEY);")},
		"0002_add_name.sql": &fstest.MapFile{Data: []byte("ALTER TABLE widgets ADD COLUMN name text;")},
	}
	if err := migrateFS(db, fsys); err != nil {
		t.Fatalf("migrateFS: %v", err)
	}

	if !db.Migrator().HasTable("widgets") || !db.Migrator().HasColumn("widgets", "name") {
		t.Fatal("fresh database did not run every migration")
	}
	var count int64
	if err := db.Table(migrateTable).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("schema_migrations rows = %d, want 2", count)
	}
}

func TestMigrateSchemaMatchesAutoMigrate(t *testing.T) {
	migrated := openRawDB(t)
	if err := Migrate(migrated, testModels()...); err != nil {
		t.Fatal(err)
	}

	auto := openRawDB(t)
	if err := auto.AutoMigrate(testModels()...); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"users", "comments", "operation_logs", "refresh_tokens"} {
		got := tableInfo(t, migrated, table)
		want := tableInfo(t, auto, table)
		if diff := compareColumns(got, want); diff != "" {
			t.Errorf("%s schema drift:\n%s", table, diff)
		}
		if diff := compareIndexes(tableIndexes(t, migrated, table), tableIndexes(t, auto, table)); diff != "" {
			t.Errorf("%s index drift:\n%s", table, diff)
		}
	}

	var fkCount int
	if err := migrated.Raw("SELECT count(*) FROM pragma_foreign_key_list('comments')").Scan(&fkCount).Error; err != nil {
		t.Fatal(err)
	}
	if fkCount != 1 {
		t.Fatalf("comments foreign keys = %d, want 1", fkCount)
	}
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	db := openRawDB(t)
	fsys := fstest.MapFS{
		"0001_bad.sql": &fstest.MapFile{Data: []byte(
			"CREATE TABLE rolled_back (id integer);\nINSERT INTO missing_table VALUES (1);",
		)},
	}
	if err := migrateFS(db, fsys); err == nil {
		t.Fatal("invalid migration should fail")
	}
	if db.Migrator().HasTable("rolled_back") {
		t.Fatal("failed migration must roll back its statements")
	}
	var count int64
	if err := db.Table(migrateTable).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("schema_migrations rows = %d, want 0 (failed version must not be recorded)", count)
	}
}

func TestMigrateLegacyRequiresModels(t *testing.T) {
	db := openRawDB(t)
	if err := db.AutoMigrate(testModels()...); err != nil {
		t.Fatal(err)
	}
	if err := migrateFS(db, fstest.MapFS{}); err == nil {
		t.Fatal("legacy database without models should be rejected")
	}
}

func TestMigrationFilesFiltersNonSQLAndDirectories(t *testing.T) {
	fsys := fstest.MapFS{
		"0002_ok.sql": &fstest.MapFile{Data: []byte("CREATE TABLE ok (id integer);")},
		"notes.txt":   &fstest.MapFile{Data: []byte("not sql")},
		"sub/x.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE nested (id integer);")},
	}
	files, err := migrationFiles(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].version != "0002_ok" {
		t.Fatalf("files = %+v, want only 0002_ok", files)
	}
}

func TestMigrateFailsWhenVersionCannotBeRecorded(t *testing.T) {
	db := openRawDB(t)
	if err := db.Exec(fmt.Sprintf(
		"CREATE TABLE %s (version TEXT PRIMARY KEY, applied_at DATETIME NOT NULL)", migrateTable,
	)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(fmt.Sprintf(
		"CREATE TRIGGER block_version BEFORE INSERT ON %s BEGIN SELECT RAISE(ABORT, 'blocked'); END",
		migrateTable,
	)).Error; err != nil {
		t.Fatal(err)
	}

	fsys := fstest.MapFS{
		"0001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE should_rollback (id integer);")},
	}
	if err := migrateFS(db, fsys); err == nil {
		t.Fatal("version record failure should fail the migration")
	}
	if db.Migrator().HasTable("should_rollback") {
		t.Fatal("version record failure must roll back the schema statement")
	}
}

func TestSplitStatements(t *testing.T) {
	body := "-- comment\nCREATE TABLE a (id integer);\n\nCREATE INDEX i ON a(id);\n"
	got := splitStatements(body)
	if len(got) != 2 {
		t.Fatalf("statements = %#v, want 2", got)
	}
}

// TestSplitStatements_KeepsSemicolonInsideLiteralAndComment 分号出现在字符串、
// 带引号标识符或块注释里不是语句边界。按裸分号切会把一条 INSERT 切成两条非法 SQL，
// 迁移在部署机上失败——而这只有等到真实数据用到分号时才暴露。
func TestSplitStatements_KeepsSemicolonInsideLiteralAndComment(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"单引号字符串", "INSERT INTO t(v) VALUES ('a;b');", 1},
		{"字符串里的转义单引号", "INSERT INTO t(v) VALUES ('it''s; fine');", 1},
		{"双引号标识符", "SELECT \"we;ird\" FROM t;", 1},
		{"反引号标识符", "SELECT `we;ird` FROM t;", 1},
		{"块注释", "/* a;b */\nSELECT 1;", 1},
		{"行注释里的分号", "SELECT 1 -- a;b\n;", 1},
		{"真分号仍然切分", "INSERT INTO t(v) VALUES ('a;b');SELECT 2;", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := splitStatements(c.body); len(got) != c.want {
				t.Fatalf("statements = %#v, want %d 条", got, c.want)
			}
		})
	}
}

// TestMigrateKeepsSemicolonInsideLiteral 从迁移执行的结果验证同一条契约：
// 字面量里的分号必须原样入库，而不是把语句切坏后报错。
func TestMigrateKeepsSemicolonInsideLiteral(t *testing.T) {
	db := openRawDB(t)
	fsys := fstest.MapFS{
		"0001_semicolon.sql": &fstest.MapFile{Data: []byte(
			"CREATE TABLE phrases (text TEXT NOT NULL);\n" +
				"INSERT INTO phrases(text) VALUES (';a;b');\n",
		)},
	}
	if err := migrateFS(db, fsys); err != nil {
		t.Fatalf("migrateFS: %v", err)
	}
	var stored string
	if err := db.Table("phrases").Select("text").Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != ";a;b" {
		t.Fatalf("stored = %q, want %q", stored, ";a;b")
	}
}

type columnInfo struct {
	Name      string
	Type      string
	NotNull   int
	DfltValue sql.NullString
	PK        int
}

func tableInfo(t *testing.T, db *gorm.DB, table string) []columnInfo {
	t.Helper()
	var cols []columnInfo
	if err := db.Raw(fmt.Sprintf("PRAGMA table_info(`%s`)", table)).Scan(&cols).Error; err != nil {
		t.Fatal(err)
	}
	sort.Slice(cols, func(i, j int) bool { return cols[i].Name < cols[j].Name })
	return cols
}

// indexInfo 索引的结构事实：名字、唯一性、列顺序。
//
// 只比名字挡不住「UNIQUE 被删掉但名字还在」这类漂移；迁移库与 AutoMigrate 库
// 必须是同一个索引定义，而不只是同一批索引名。
type indexInfo struct {
	Name    string
	Unique  int
	Columns string
}

func tableIndexes(t *testing.T, db *gorm.DB, table string) []indexInfo {
	t.Helper()
	var raw []struct {
		Name   string
		Unique int
	}
	if err := db.Raw(fmt.Sprintf("PRAGMA index_list(`%s`)", table)).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}

	out := make([]indexInfo, 0, len(raw))
	for _, idx := range raw {
		if strings.HasPrefix(idx.Name, "sqlite_") {
			continue
		}
		var cols []struct {
			Name string
		}
		if err := db.Raw(fmt.Sprintf("PRAGMA index_info(`%s`)", idx.Name)).Scan(&cols).Error; err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(cols))
		for _, col := range cols {
			names = append(names, col.Name)
		}
		out = append(out, indexInfo{Name: idx.Name, Unique: idx.Unique, Columns: strings.Join(names, ",")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func compareColumns(got, want []columnInfo) string {
	if len(got) != len(want) {
		return fmt.Sprintf("column count %d != %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Sprintf("column %q = %#v, want %#v", want[i].Name, got[i], want[i])
		}
	}
	return ""
}

func compareIndexes(got, want []indexInfo) string {
	if len(got) != len(want) {
		return fmt.Sprintf("index count %d != %d: %v vs %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Sprintf("index %q = %#v, want %#v", want[i].Name, got[i], want[i])
		}
	}
	return ""
}
