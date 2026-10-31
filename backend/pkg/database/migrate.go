package database

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"ebook-server/migrations"

	"gorm.io/gorm"
)

// migrateTable 记录已执行的迁移版本。
const migrateTable = "schema_migrations"

// legacyTables 是 AutoMigrate 时代已经存在的表；任一存在即视为遗留库。
var legacyTables = []string{"users", "comments", "operation_logs", "refresh_tokens"}

type migrationFile struct {
	version string
	name    string
}

// Migrate 执行未应用的迁移。
//
// 新库按文件名顺序执行嵌入的 SQL；遗留库（已有业务表、但 schema_migrations 为空）
// 先对 models 跑一次 AutoMigrate 收养。
//
// 收养后必须把**当前二进制内嵌的全部版本**标记为已应用，而不是只标记 0001：
// AutoMigrate 已把库带到当前 models 的最新 schema，仍标记为未应用的未来迁移会在
// 下一次启动时重放，`ALTER TABLE ... ADD COLUMN` 之类立即因列已存在而失败——
// 恰好砸在收养机制要保护的升级路径上。代价是存量库不会再执行当前版本中的数据迁移；
// 当前迁移只有 schema 变更，引入数据迁移前必须重新设计收养（见 ADR-0023）。
func Migrate(db *gorm.DB, models ...interface{}) error {
	return migrateFS(db, migrations.FS, models...)
}

// migrateFS 是 Migrate 的可注入文件系统版本：测试用内存 FS 模拟「未来迁移」，
// 不需要为了验证收养路径往真实 migrations 目录里放一次性 SQL。
func migrateFS(db *gorm.DB, fsys fs.FS, models ...interface{}) error {
	if err := db.Exec(fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (version TEXT PRIMARY KEY, applied_at DATETIME NOT NULL)",
		migrateTable,
	)).Error; err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}

	files, err := migrationFiles(fsys)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		legacy, err := hasLegacyTables(db)
		if err != nil {
			// 判定失败按 fail-closed 处理：误判为「新库」会让遗留库跳过收养，
			// 从此缺新列且永不补齐，宁可拒绝启动。
			return fmt.Errorf("detect legacy schema: %w", err)
		}
		if legacy {
			if len(models) == 0 {
				return fmt.Errorf("legacy database requires models for baseline adoption")
			}
			if err := db.AutoMigrate(models...); err != nil {
				return fmt.Errorf("adopt legacy schema: %w", err)
			}
			return markAllApplied(db, files)
		}
	}

	for _, file := range files {
		if applied[file.version] {
			continue
		}
		if err := applyMigration(db, fsys, file); err != nil {
			return err
		}
	}
	return nil
}

func migrationFiles(fsys fs.FS) ([]migrationFile, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var files []migrationFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, migrationFile{
			version: strings.TrimSuffix(entry.Name(), ".sql"),
			name:    entry.Name(),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}

func applyMigration(db *gorm.DB, fsys fs.FS, file migrationFile) error {
	body, err := fs.ReadFile(fsys, file.name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", file.name, err)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range splitStatements(string(body)) {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("apply %s: %w", file.name, err)
			}
		}
		if err := recordMigration(tx, file.version, time.Now().UTC()); err != nil {
			return err
		}
		return nil
	})
}

// splitStatements 切分 SQL，只把**字符串字面量、引号标识符与注释之外**的分号当作语句边界。
//
// 按裸分号切分听着够用，实际会在第一条带分号的数据上把一条 INSERT 切成两条非法 SQL，
// 而失败只出现在真实数据跑起来的那台部署机上。这里按 SQL 词法走一遍：定界符内原样
// 保留（含 SQL 的双写转义 `''`），`--` 到行尾、`/* */` 到闭合为止整体丢弃。
// 逐字节扫描对 UTF-8 安全——多字节字符的后续字节都 >= 0x80，不会撞上这些 ASCII 边界符。
func splitStatements(body string) []string {
	var out []string
	var buf []byte

	for i := 0; i < len(body); {
		switch {
		case body[i] == '\'' || body[i] == '"' || body[i] == '`':
			quote := body[i]
			buf = append(buf, quote)
			i++
			for i < len(body) {
				if body[i] == quote {
					// 双写的同名定界符是内容里的一个引号，不是闭合（反引号标识符没有这条规则）
					if quote != '`' && i+1 < len(body) && body[i+1] == quote {
						buf = append(buf, quote, quote)
						i += 2
						continue
					}
					buf = append(buf, quote)
					i++
					break
				}
				buf = append(buf, body[i])
				i++
			}
		case i+1 < len(body) && body[i] == '-' && body[i+1] == '-':
			for i < len(body) && body[i] != '\n' {
				i++
			}
		case i+1 < len(body) && body[i] == '/' && body[i+1] == '*':
			i += 2
			for i+1 < len(body) && !(body[i] == '*' && body[i+1] == '/') {
				i++
			}
			if i+1 < len(body) {
				i += 2
			} else {
				i = len(body)
			}
			buf = append(buf, ' ') // 注释换成空白，免得两侧的词粘在一起
		case body[i] == ';':
			if stmt := strings.TrimSpace(string(buf)); stmt != "" {
				out = append(out, stmt)
			}
			buf = buf[:0]
			i++
		default:
			buf = append(buf, body[i])
			i++
		}
	}

	if stmt := strings.TrimSpace(string(buf)); stmt != "" {
		out = append(out, stmt)
	}
	return out
}

func appliedVersions(db *gorm.DB) (map[string]bool, error) {
	var versions []string
	if err := db.Table(migrateTable).Pluck("version", &versions).Error; err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	applied := make(map[string]bool, len(versions))
	for _, v := range versions {
		applied[v] = true
	}
	return applied, nil
}

// markAllApplied 把当前二进制内嵌的版本一次性记为已应用（收养专用）。
//
// 放在一个事务里：要么整组版本都落下，要么一个都不落。否则部分版本已标记时，
// 下一次启动会走「正常迁移」路径，把剩下的迁移重新执行到 AutoMigrate 已更新的库上。
func markAllApplied(db *gorm.DB, files []migrationFile) error {
	if len(files) == 0 {
		return nil
	}
	now := time.Now().UTC()
	return db.Transaction(func(tx *gorm.DB) error {
		for _, file := range files {
			if err := recordMigration(tx, file.version, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// recordMigration 在事务内登记一个已应用的版本。
func recordMigration(tx *gorm.DB, version string, appliedAt time.Time) error {
	if err := tx.Exec(
		fmt.Sprintf("INSERT INTO %s(version, applied_at) VALUES (?, ?)", migrateTable),
		version, appliedAt,
	).Error; err != nil {
		return fmt.Errorf("record %s: %w", version, err)
	}
	return nil
}

// hasLegacyTables 任一业务表存在即视为遗留库。
//
// 表存在性走 GORM 的 Migrator 而不是直接查 sqlite_master：后者是 SQLite 系统表的
// 方言细节，这道判定不该绑死在它上面（防御性写法，不是多库承诺——迁移 SQL 本身
// 按 SQLite 方言书写，多库支持不在范围内，见 ADR-0023）。Migrator 吞掉驱动错误，
// 所以先做一次 `SELECT 1` 探活：连不上就 fail-closed 拒绝启动，不带着半迁移的库
// 继续服务。
func hasLegacyTables(db *gorm.DB) (bool, error) {
	var probe int
	if err := db.Raw("SELECT 1").Scan(&probe).Error; err != nil {
		return false, fmt.Errorf("probe database: %w", err)
	}
	for _, table := range legacyTables {
		if db.Migrator().HasTable(table) {
			return true, nil
		}
	}
	return false, nil
}
