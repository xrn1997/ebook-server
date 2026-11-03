# 测试库与生产同走版本化迁移

Status: accepted

测试库建表不再走 AutoMigrate，与生产同调 `backend/pkg/database` 的 `Migrate`：schema 真相源收口到迁移文件一处，AutoMigrate 只剩收养路径内部用途。

## 背景

ADR-0023 之后 schema 仍有三份手写副本：model 结构、`backend/migrations` 的基线与增量、`sql/init.sql`（已删）的 MySQL 参考翻译。三份副本靠对照测试同步：测试库经 AutoMigrate 建出「当前 schema」，再与迁移文件、参考脚本逐一比对。问题在于对照物本身经 AutoMigrate 中转——三份手写副本无主人，同步靠事后追认；AutoMigrate 退出生产建表后仍把持测试建表，模型加了列而迁移漏写时，测试库照样建出新列，漂移不会变红。

## 决定

- `pkg/testdb` 的 `Open` 改调 `database.Migrate`，测试库与生产走同一条迁移路径；模型清单自持在 `Migrate` 内部，调用方不再传。
- 连接由 `database.OpenMemory` 给出：按连接生效的 pragma（`busy_timeout`/`foreign_keys`）在内存库上同样附加，测试库不再跑在比生产宽松的库里。WAL 只属于文件库——内存库没有 WAL 可言。
- AutoMigrate 只剩收养路径内部用途：遗留库（已有业务表、`schema_migrations` 为空）的基线收养仍由 `Migrate` 内部对内置清单跑一次 AutoMigrate——收养语义不变（ADR-0023）。
- `sql/init.sql`（已删）参考脚本删除；MySQL 方言参考归 `docs/development.md` 一句话，不再维护脚本副本。

## 边界

- 生产迁移语义零变化：迁移文件的执行顺序与收养标记逻辑不变（模型清单改由 `Migrate` 自持、调用方不再传，属 ADR-0023 的范围，不改迁移语义）；收养标记全部版本、数据迁移前重新设计收养、不引入第三方迁移库等边界仍归 ADR-0023。
- 收养路径语义零变化（见上一条决定）。
- 本决定不构成多数据库支持；迁移 SQL 仍按 SQLite 方言书写（ADR-0023）。

## 后果

- 对照测试失去对照物，随之删除：`TestMigrateSchemaMatchesAutoMigrate`（迁移库 vs AutoMigrate 库）、`TestSqlInitMatchesMigrations` 与 `TestSqlInitHasNoStaleIndexNames`（参考脚本 vs 当前 schema）。
- 测试库获得与生产相同的迁移语义：漏写迁移文件会在测试中以缺列暴露，而不是被 AutoMigrate 静默补齐；`backend/pkg/testdb` 的 `TestOpenRunsMigrations` 钉住这条路径。
- 对照测试删除后留下的漂移面由**性质断言**接管，不再靠评审把关：`backend/pkg/database` 的 `TestMigratedSchemaKeepsItsConstraints` 直接对内嵌迁移的产物钉主键列、唯一索引列集、显式创建索引的名字与列序、NOT NULL 列集与外键（`comments.user_id → users.uid`）；`TestMigratedUniqueIndexRejectsDuplicateEmail` 与 `pkg/testdb` 的 `TestOpenEnforcesForeignKeys` 钉这些约束**在生效**，而不是只在 DDL 里存在——注册用例走应用层预检，唯一索引被删掉它照样绿，所以约束的行为必须单独钉。普通索引没有这种运行时判据：没有任何用例能观察到一个索引不存在，删掉它只是让查询静默退化成全表扫描，所以它的看守只能是那份按名字与列序写出的期望表。期望表按 `0001_init.sql` 逐条写出，不跟着模型走；新增、改名或改窄索引时，期望表随迁移同批更新。
- 仍然存在的残留漂移面：模型与迁移**同时**朝错误方向改（两边都漏了同一列/同一条索引）不会变红。这是把清单自持、取消第二份对照物的必然代价，不是本决定新引入的。
- schema 真相源从三份副本收口到 `backend/pkg/database` 一个主人。

## 被否方案

- **基线 SQL 由模型生成后入库**：引入生成器工具链，多一份生成物与生成步骤，代价大于收益；否决。
- **维持双路径靠对照测试同步**：三份手写副本无主人，同步靠事后追认——本决策要消灭的正是它；否决。

锚点：`backend/pkg/testdb` 的 `Open` 与钉住它的 `TestOpenRunsMigrations`、`TestOpenEnforcesForeignKeys`；`backend/pkg/database` 的 `Migrate`、`OpenMemory` 与 `TestMigratedSchemaKeepsItsConstraints`；`docs/adr/0023-versioned-migrations.md`。
