# 版本化迁移采用嵌入 SQL + 基线收养

Status: accepted

生产启动走 `backend/pkg/database` 的 `Migrate`：新库按文件名顺序执行 `backend/migrations/*.sql`；已有业务表但 `schema_migrations` 为空时，先对传入的 models 跑一次 AutoMigrate 收养，再把**当前二进制内嵌的全部版本**标记为已应用。后续 schema 变更必须新增迁移文件，不再依赖启动时的 AutoMigrate。

## 边界

- 测试库仍用 `pkg/testdb` 的 AutoMigrate；它只服务测试，不是生产迁移路径。
- 基线 `0001_init.sql` 只覆盖当前 schema；新增列/索引必须另写迁移。
- 迁移 SQL 按 **SQLite 方言**书写（`AUTOINCREMENT`、反引号均可）：本机制只承诺 SQLite，`migrate.go` 里方言中立的表存在性检查是防御性写法、不构成多库支持；`sql/init.sql` 是 MySQL 方言的参考翻译，不参与执行。
- 收养为什么标记全部版本：AutoMigrate 已经把库带到当前 models 的最新 schema；若只标记 `0001_init`，未来迁移会在下一次启动时重放，非幂等 `ALTER TABLE` 直接失败——恰好砸在收养机制要保护的升级路径上。代价是存量库不会重放当前版本中的数据迁移；引入数据迁移前必须重新设计收养（例如版本化模型快照）。
- 迁移在启动时执行，失败即退出；不带着半迁移的库继续服务。
- 不引入 goose/golang-migrate；当前只有顺序 SQL + 版本表，标准库 `embed` 足够。需要 down/out-of-order 时再升级。

## 被否方案

- **继续只用 AutoMigrate**：没有版本记录、无法审阅结构变更，也无法保证旧库与新库走到同一 schema；否决。
- **引入第三方迁移库**：为一个顺序执行的基线增加依赖与方言适配；当前收益不足，否决。
- **只建 `schema_migrations` 不收养旧库**：AutoMigrate 时代的库会缺列，升级直接失败；否决。
- **收养只标记 0001、继续重放后续迁移**：AutoMigrate 已把库更新到最新 schema，重放非幂等迁移必然失败；否决。

锚点：`backend/migrations`；`backend/pkg/database` 的 `Migrate`；`backend/pkg/database/migrate_test.go`；`backend/main.go`。
