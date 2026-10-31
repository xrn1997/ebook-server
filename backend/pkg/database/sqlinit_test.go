package database

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// sqlInitPath 是 MySQL 参考脚本的位置（仓库根的 sql/init.sql）。
//
// 它不是 schema 的事实源，只是把当前结构翻成 MySQL 方言的参考——正因如此，
// 它必须与事实源对齐：真正的 MySQL 迁移会照着它写，参考脚本漏一列，
// 照着它建出来的库就会缺一列。
var sqlInitPath = filepath.Join("..", "..", "..", "sql", "init.sql")

var (
	// createTableRe 抓出建表语句的表名与表体。
	createTableRe = regexp.MustCompile("(?is)CREATE TABLE IF NOT EXISTS\\s+`?([A-Za-z_]+)`?\\s*\\((.*?)\\n\\)")
	// backtickColumnRe 抓 SQLite 风格的带反引号列名。
	backtickColumnRe = regexp.MustCompile("^`([A-Za-z_]+)`\\s")
	// indexRe 抓索引名。两种方言的写法不同：
	//   SQLite：CREATE [UNIQUE] INDEX IF NOT EXISTS `name` ON ...
	//   MySQL： 建表体内联的 UNIQUE INDEX name (...) / INDEX name (...)
	indexRe = regexp.MustCompile("(?i)INDEX\\s+(?:IF\\s+NOT\\s+EXISTS\\s+)?`?([A-Za-z_]+)`?")
)

// 建表体里不是列的那些行首关键字。
var nonColumnKeywords = map[string]bool{
	"UNIQUE":     true,
	"INDEX":      true,
	"KEY":        true,
	"CONSTRAINT": true,
	"PRIMARY":    true,
	"FOREIGN":    true,
	"CHECK":      true,
}

// TestSqlInitMatchesMigrations MySQL 参考脚本的列集合必须与迁移文件一致。
//
// sql/init.sql 曾长期落后于模型：缺 comment_key、chapter_url、error_code 等列，
// 索引名也与模型对不上。文件头写着「不是事实源」，但照着它做 MySQL 迁移的人
// 只会照抄它——参考脚本错了，错误就会一路带到新库。
//
// 对照的是 AutoMigrate 建出的当前 schema，而不是只读 0001_init.sql：迁移与
// AutoMigrate 的一致性由 TestMigrateSchemaMatchesAutoMigrate 钉住；若这里的输入
// 仍是基线文件，加入第一个后续迁移后参考脚本就再也不会因此变红。
func TestSqlInitMatchesMigrations(t *testing.T) {
	current := currentSchemaColumns(t)

	source, err := os.ReadFile(sqlInitPath)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", sqlInitPath, err)
	}
	reference := parseCreateTables(string(source))

	for table, want := range current {
		got, ok := reference[table]
		if !ok {
			t.Errorf("%s 里没有表 %s", sqlInitPath, table)
			continue
		}
		var missing, extra []string
		for column := range want {
			if !got[column] {
				missing = append(missing, column)
			}
		}
		for column := range got {
			if !want[column] {
				extra = append(extra, column)
			}
		}
		if len(missing) > 0 {
			t.Errorf("表 %s 缺列 %v（与迁移文件不一致）", table, missing)
		}
		if len(extra) > 0 {
			t.Errorf("表 %s 多出迁移文件里没有的列 %v", table, extra)
		}
	}

	for table := range reference {
		if _, ok := current[table]; !ok {
			t.Errorf("%s 里的表 %s 在迁移文件里不存在", sqlInitPath, table)
		}
	}
}

// currentSchemaColumns 用 AutoMigrate 建出当前 schema，读出「表 → 列集合」。
//
// sql/init.sql 的对照对象是 model 结构中的当前事实源：迁移与 AutoMigrate 之间由
// TestMigrateSchemaMatchesAutoMigrate 对齐，因此这里再用 AutoMigrate 作输入，
// 后续迁移新增的列也会被这条对照看见。
func currentSchemaColumns(t *testing.T) map[string]map[string]bool {
	t.Helper()

	db := openRawDB(t)
	if err := db.AutoMigrate(testModels()...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	tables, err := db.Migrator().GetTables()
	if err != nil {
		t.Fatalf("GetTables: %v", err)
	}

	columns := map[string]map[string]bool{}
	for _, table := range tables {
		if strings.HasPrefix(table, "sqlite_") {
			continue
		}
		types, err := db.Migrator().ColumnTypes(table)
		if err != nil {
			t.Fatalf("ColumnTypes(%s): %v", table, err)
		}
		set := map[string]bool{}
		for _, column := range types {
			set[column.Name()] = true
		}
		columns[strings.ToLower(table)] = set
	}
	return columns
}

// parseCreateTables 从建表语句里抽出「表 → 列集合」。
//
// 两种方言的列名写法不同：SQLite 迁移文件用反引号包住，MySQL 脚本裸写。
// 两者都按「每行一条列定义」解析；行首是保留字（UNIQUE / INDEX / CONSTRAINT…）
// 的行不是列。
func parseCreateTables(sql string) map[string]map[string]bool {
	tables := map[string]map[string]bool{}
	for _, match := range createTableRe.FindAllStringSubmatch(sql, -1) {
		columns := map[string]bool{}
		for _, line := range strings.Split(match[2], "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "--") {
				continue
			}
			if m := backtickColumnRe.FindStringSubmatch(line); m != nil {
				columns[m[1]] = true
				continue
			}
			name, _, _ := strings.Cut(line, " ")
			if name == "" || nonColumnKeywords[strings.ToUpper(name)] {
				continue
			}
			columns[name] = true
		}
		tables[strings.ToLower(match[1])] = columns
	}
	return tables
}

// TestSqlInitHasNoStaleIndexNames 参考脚本里的索引名必须与模型一致。
//
// 列名对得上但索引名对不上，同样会让照抄的人建出不同的索引（唯一索引与普通
// 索引的区别尤其致命）。对照对象与列检查一致：AutoMigrate 的当前 schema。
func TestSqlInitHasNoStaleIndexNames(t *testing.T) {
	source, err := os.ReadFile(sqlInitPath)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", sqlInitPath, err)
	}
	want := currentIndexNames(t)

	var got []string
	for _, match := range indexRe.FindAllStringSubmatch(string(source), -1) {
		got = append(got, match[1])
	}
	sort.Strings(got)

	for _, name := range got {
		if !want[name] {
			t.Errorf("%s 里的索引 %s 在迁移文件里不存在", sqlInitPath, name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%s 里有 %d 个索引, 迁移文件里有 %d 个：%v vs %v",
			sqlInitPath, len(got), len(want), got, want)
	}
}

// currentIndexNames 用 AutoMigrate 建出当前 schema，读出全部索引名。
func currentIndexNames(t *testing.T) map[string]bool {
	t.Helper()

	db := openRawDB(t)
	if err := db.AutoMigrate(testModels()...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	tables, err := db.Migrator().GetTables()
	if err != nil {
		t.Fatalf("GetTables: %v", err)
	}

	want := map[string]bool{}
	for _, table := range tables {
		if strings.HasPrefix(table, "sqlite_") {
			continue
		}
		for _, idx := range tableIndexes(t, db, table) {
			want[idx.Name] = true
		}
	}
	return want
}
