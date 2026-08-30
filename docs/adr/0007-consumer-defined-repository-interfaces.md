# repository 接口由消费方定义，测试替身用 SQLite :memory:

service 层通过**定义在 service 包内的 Store 接口**访问数据，repository 是满足这些接口的
gorm adapter。依赖全部由构造函数注入，`main.go` 是唯一装配点。测试替身使用独立的
SQLite `:memory:` 实例（同一 gorm 实现、独立状态），**不引入手写 mock**。

## 动机

* **此前的「注入」是假的**：`AGENTS.md` 曾声称「依赖注入：通过函数参数传递依赖」，但实际是
  构造函数零参数、自行向下 `new`，repository 是空 `struct{}`、每个方法内部抓取全局
  `database.DB`。service 无法脱离全局状态测试，三份几乎相同的测试 DB 初始化散落在
  repository / service / handler 三处。

* **接口宽度应由消费方决定**：接口定义在 repository 侧会跟着实现走，容易长成
  「实现有几个公开方法接口就有几个」——正是要治的浅。定义在 service 侧，接口只列
  service 真正调用的方法，加新方法时不会为了「完整性」而膨胀。

* **:memory: 优于手写 mock**：repository 的实现是 GORM 查询语义（`Preload`、`Count` 分页、
  软删过滤）。手写 fake 必须在测试里重新实现这些语义，一旦与 GORM 行为漂移，
  测试会对你说谎——绿着但生产错。独立的 `:memory:` 实例跑的是真 SQL，测试结果可信；
  代价（毫秒级启动）在本项目体量下可忽略。

## 决策

1. **接口定义在 `service/ports.go`**：`UserStore` / `TokenStore` / `CommentStore` / `LogStore`
   四个，方法集只含 service 实际调用的方法。零调用方法不进接口（`UserRepository.Delete`
   软删除因此被删除——它与 ADR-0005「不软删」冲突，是危险死代码）。
2. **repository 构造函数接收 `*gorm.DB`**，不再读取 `database.DB` 全局；`pkg/database`
   保留 `Init`/`GetDB`，仅供 `main.go` 装配时使用。
3. **测试替身 = `pkg/testdb.Open`**：每个测试独立 `:memory:` 库（强制单连接，
   避免 SQLite `:memory:` 每连接独立库的池化陷阱），替代三份重复的全局改写式初始化。
4. **`repository.IsRecordNotFound` 保留**：service 层 12 处使用它是 service 仍 import
   repository 的唯一理由。gorm 错误到领域 sentinel 的翻译留待需要时再做
   （见「被拒方案」）。
5. **`Mailer` 接口同样定义在 service 包**，adapter 有三个：SMTP（生产）、写日志
   （本地联调降级）、记录型 fake（测试）。装配选择发生在 `main.go`。
6. **验证码存储与限流器不做接口**：`code.Store`、`ratelimit.Limiter` 以具体类型注入/
   随实例创建。Redis 版验证码存储（ADR-0001 已知问题）是未来的第二个 adapter，
   到那时再立 seam——只有一个 adapter 时不建接口。

## 被拒方案

* **接口定义在 repository 包**：与实现 1:1 演化，接口宽度失控（见动机）。

* **手写 in-memory fake 做 service 测试替身**：`Preload` / 分页 / 软删过滤语义重实现
  成本高且必然漂移（见动机）。repository 测试继续用真 SQL，同样理由。

* **adapter 在 seam 上把 gorm `ErrRecordNotFound` 翻译成领域 sentinel**（消灭 service
  对 repository 的最后依赖）：能做，但要动 12 处调用与对应测试。收益是「service 无法
  直接 new repository」的编译期保证，代价是撑大本次改动面。当前靠约定守住，
  需要时再单独立项。

## 下游影响

* `service/ports.go`：新增四个 Store 接口与 `Mailer` 接口。
* `repository/*`：构造函数接收 `*gorm.DB`；删除零调用的 `UserRepository.Delete` 与
  `HasError`；`DeleteExpired` 保留在具体类型上（对应「refresh token 无定时清理」已知
  问题，暂不进接口）。
* `main.go`：所有 service / handler 的组装集中于此。
* `handler/*`：构造函数接收 service 实例。
* 测试：`pkg/testdb` 统一建库；handler 测试通过 `newTestApp` 组装独立的一整套依赖。
* `AGENTS.md`：依赖注入与测试约定的描述按本文修正。

## 验收

1. `go build` / `go vet` / `gofmt -l` / `go test -count=1 ./...` 全绿。
2. 生产代码中除 `main.go` 外不出现 `database.GetDB()`；`code.Default()` 不再存在。
3. 任何 API 的请求/响应与改造前完全一致（本决策为纯内部结构变更）。
