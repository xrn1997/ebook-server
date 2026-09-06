# 评论聚合键迁移、多键并集查询与后台管理面扩展

为「合并书籍」这一客户端场景补齐评论域能力：用户在第三方书源改版（章节 URL 变化、
多本书合并）后，需要把自己名下旧聚合键的评论批量迁到新键；后台管理面则需要
搜索、详情、删除与筛选，才能在桌面应用里真正管理数据。设计文档
`docs/superpowers/specs/2026-09-05-desktop-app-design.md` 与
[ADR-0012](0012-desktop-admin-app-sidecar.md) 都声称「Go 后端零改动即可被桌面应用带动」，
本 ADR 记录这句话被推翻的部分及其原因，避免后来者再把「零改动」当成硬约束。

## 决策

### 1. `POST /api/comments/migrate-key`：聚合键批量迁移，仅限本人

* 请求体 `{old_key, new_key}`，两键都对应 `comment.chapter_url`；只迁移当前用户名下的评论，
  返回 `{migrated_count}`。
* **空串是合法键**：`chapter_url` 的空串即书籍级评论（ADR-0011），因此两键用
  `omitempty` 而非 `required`——「书籍级 → 章节」与「章节 → 书籍级」两个方向都必须可表达。
  binding tag 无法拼常量，读路径（公开列表过滤）与写路径的 `max=2048` 由
  `TestChapterURLLengthLimitMatchesModelBinding` 用反射钉住一致。
* `old_key == new_key`（含两键同空）返回 `A0305`；无匹配行返回 0 而不报错——合并流程会重试，
  报错会让客户端无法区分「已生效」与「失败」。
* 不迁移 `chapter_name/book_name`：它们是展示快照，聚合语义只看 `chapter_url`（ADR-0011）。

### 2. `GET /api/comments?chapter_url=` 支持多键返回并集

合并书籍常涉及多个旧章节键；单键等值查询并入 `chapter_url IN ?`（单元素 IN 与等值等价，
因此 `FindByChapter` 不再单独存在）。公开端点加两条上限：单键 ≤2048、键数 ≤50——
每个键展开成一个绑定变量，SQLite 单语句变量数默认上限 999，超限直接报错落成 C0500，
而这是无需登录的端点，不限长度即可被任意人打满。

### 3. 后台管理面补齐搜索 / 详情 / 删除 / 筛选

设计文档 §页面清单要求「用户：列表、搜索、详情；评论：列表、搜索、删除；日志：列表、筛选」，
而既有后台只有三个分页全量列表。据此新增（全部在 ADR-0010 的独立后台鉴权之后）：

| 端点 | 能力 |
| --- | --- |
| `GET /admin/api/users?keyword=` | 关键字模糊匹配邮箱/用户名/昵称，纯数字兼命中 UID；空关键字即全量 |
| `GET /admin/api/users/:uid` | 用户详情（资料 + 该用户评论数） |
| `GET /admin/api/comments?keyword=&book_name=` | 内容关键字 + 书名过滤 |
| `DELETE /admin/api/comments/:id` | 后台治理删除（软删除），不做归属校验 |
| `GET /admin/api/logs?method=&path=&user_id=&error_code=&failed=` | 日志筛选；`failed` 只看业务码非 00000 且非空的请求 |

配套约束：

* 管理面接口仍由 consumer-defined 接口定义（ADR-0007），因评论接口含写操作，
  `XxxReader` 更名为 `XxxStore`。
* LIKE 关键字统一经 `repository.likePattern` 转义 `%`/`_`/`\`：搜索框里的 `%`
  应当是普通字符，不是「匹配全部」。
* 「先 Count 再取页」的分页形状收敛为 `repository.paginateQuery`，新增列表维度不再复制四行套路。

## 被拒方案

* **迁移做成后台专属端点**：迁移的主体是客户端用户（合并书籍发生在阅读器里），
  放公开 API 走用户 JWT 才符合「仅本人」的归属模型。
* **迁移时同步改写展示快照**：快照的语义是「用户当时的所见」，批量改写反而伪造历史。
* **给迁移加事务**：单条 `UPDATE ... WHERE user_id = ? AND chapter_url = ?` 本身原子，
  无跨表一致性需求。
* **后台删除保留归属校验**：管理员代用户删违规内容正是这个端点的定义，
  校验归属等于没给治理能力。

## 下游影响

* `desktop/` 的用户/评论/日志三个视图据此前提实现搜索、分页（页长固定 20）、
  详情面板、评论删除（两步确认）与日志筛选。
* 后端启动改为真优雅关闭（`signal.Notify` + `http.Server.Shutdown` + stdin `shutdown`
  指令），桌面端停止不再耗尽 5 秒强杀——这是 ADR-0012 §1「请求优雅退出」承诺的兑现，
  也是「Go 零改动」主张的第二次修正。

## 验收

* `go test ./...` 覆盖：空键迁移、同键 A0305、用户隔离、幂等重试、并集查询、
  键数/键长上限、LIKE 转义、后台搜索/详情/删除/筛选。
* 公开列表在键数超 50 或单键超 2048 时返回 `A0400`，其余情况正常。
* 桌面端集成测试证明：停止耗时远小于 5 秒宽限期（真优雅退出），
  改端口重启后健康检查拨新端口。
