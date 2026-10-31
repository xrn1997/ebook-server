# repository 接口由消费方定义，测试替身用 SQLite :memory:

Status: accepted

service 层通过**定义在 service 包内的 Store 接口**访问数据，repository 是满足这些接口的 gorm adapter。依赖全部由构造函数注入，`main.go` 是唯一装配点。测试替身使用独立的 SQLite `:memory:` 实例（同一 gorm 实现、独立状态），**不引入手写 mock**。

## 动机

* **此前的「注入」是假的**：`AGENTS.md` 曾声称「依赖注入：通过函数参数传递依赖」，但实际是构造函数零参数、自行向下 `new`，repository 是空 `struct{}`、每个方法内部抓取全局 `database.DB`。service 无法脱离全局状态测试，三份几乎相同的测试 DB 初始化散落在 repository / service / handler 三处。

* **接口宽度应由消费方决定**：接口定义在 repository 侧会跟着实现走，容易长成「实现有几个公开方法接口就有几个」——正是要治的浅。定义在 service 侧，接口只列 service 真正调用的方法，加新方法时不会为了「完整性」而膨胀。

* **:memory: 优于手写 mock**：repository 的实现是 GORM 查询语义（`Preload`、`Count` 分页、软删过滤）。手写 fake 必须在测试里重新实现这些语义，一旦与 GORM 行为漂移，测试会对你说谎——绿着但生产错。独立的 `:memory:` 实例跑的是真 SQL，测试结果可信；代价（毫秒级启动）在本项目体量下可忽略。

## 决策

1. **数据访问接口定义在 `backend/service/ports.go`**：接口只列消费方真正调用的方法。零调用方法不进接口（`UserRepository.Delete` 软删除因此被删除——它与 ADR-0005「不软删」冲突，是危险死代码）。同一文件还定义 `Mailer`。

2. **repository 构造函数接收 `*gorm.DB`**，不再读取包级全局。`pkg/database` 只提供 `Open(path) (*gorm.DB, error)` 与 `Close(db)`（以及迁移入口 `Migrate`），由 `main.go` 装配时使用。配置、JWT 密钥（`jwt.TokenBox`）、数据库连接、验证码存储都按此办理：组装一份依赖不需要先有人初始化过某处全局，也不必为了拿到一枚 token 去改全局。

3. **测试替身 = `pkg/testdb.Open`**：每个测试独立 `:memory:` 库（强制单连接，避免 SQLite `:memory:` 每连接独立库的池化陷阱），替代三份重复的全局改写式初始化。

4. **「查不到」在 repository 边界翻成领域哨兵**：`repository.translateNotFound` 在 adapter 边界把 `gorm.ErrRecordNotFound` 换成本聚合的领域错误——账号 → `model.ErrAccountNotFound`、评论 → `model.ErrCommentNotFound`、刷新令牌（不存在与已过期同结果）→ `model.ErrLoginExpired`。gorm 的类型因此不出 repository，service 与后台只判领域错误，不需要再问「这个 err 是不是没查到」。

   同一个「没查到」在 repository / service / 后台各判一次是重复而非分层：后台曾为这枚判断白依赖 `service`，`Refresh` 里 `FindByUID` 的裸 gorm 错误也会一路漏到响应、落成 C0500。

5. **`Mailer` 接口同样定义在 service 包**，adapter 有三个：SMTP（生产）、写日志（本地联调降级）、记录型 fake（测试）。装配选择发生在 `main.go`。

6. **验证码存储与限流器不做接口**：`code.Store`、`ratelimit.Limiter` 以具体类型注入或随实例创建。Redis 版验证码存储（当前内存实现的多实例限制见 `docs/development.md`）是未来的第二个 adapter，到那时再立 seam——只有一个 adapter 时不建接口。

## 被拒方案

* **接口定义在 repository 包**：与实现 1:1 演化，接口宽度失控（见动机）。

* **手写 in-memory fake 做 service 测试替身**：`Preload` / 分页 / 软删过滤语义重实现成本高且必然漂移（见动机）。repository 测试继续用真 SQL，同样理由。

* **在 seam 上翻译 not-found（初版被拒，现已采纳）**：初版只换来一条编译期保证，收益不足以立这层间接；后来收益变多了——见决策 4。

## 边界

- 消掉的是**生产代码**的包级全局。`service` / `handler` 两套测试夹具仍是包级变量，靠同包默认串行才安全（见 `TESTING.md`）。
- 对外可见的变化限于文案：`model.ErrUserNotFound` 与 `model.ErrAccountNotFound` 合并为后者，两者本来就映射同一枚 A0201，合并后该码的文案全站唯一。业务码、状态码与响应结构均不变。

锚点：`backend/service/ports.go`；`backend/repository/notfound.go` 的 `translateNotFound`；`backend/pkg/testdb`；`backend/pkg/database` 的 `Open`。
