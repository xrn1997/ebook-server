# 评论删除范围与后台固定视图

Status: accepted

评论删除在公开表面与后台表面各有一条路径：作者删自己的评论（`DELETE /api/comments/:id`，
归属校验）与后台治理删（`DELETE /admin/api/comments/:id`，不校验归属，见 ADR-0013）。此前两条
路径的实现深度不同——公开侧是 `FindByID → CanDelete → Delete` 三次访问，后台侧直接拿
`CommentRepository`——由此产生三个问题：

* 「查到」被当成「删掉」：并发删除时 `RowsAffected = 0` 也回成功，客户端以为删了。
* 归属规则写在 service、删除语句写在 repository，后台 handler 又绕过 service 直接调仓储。
* 后台直接序列化 `model.Comment` 实体：实体新增字段（如作者 `email`）会自动变成对外契约。

## 决策

### 1. `model.CommentScope` 表达删除范围

`CommentScopeOfAuthor(uid)` 与 `CommentScopeOfAdmin()` 是同一条删除路径的两种范围，
与改键的 `CommentKeyScope` 同形但语义不同：改键的范围决定 `WHERE` 是否带 `user_id`，
删除的范围决定是否校验作者。`CommentScope` 的字段不导出，避免调用方绕过构造函数
拼出第三种组合。

### 2. `DeleteScoped` 以 `RowsAffected` 作为唯一的成功判据

仓储把范围直接编进条件：author scope 同时匹配 `id` 与 `user_id`，admin scope 只匹配 `id`。
一条条件 DELETE 天然是原子的：删到一行才算成功，不存在「先查后删」的窗口。

没删到行时由 `CommentService.Delete` 补一次查询来区分原因：

* 评论不存在 → `model.ErrCommentNotFound`（A0304）；
* 存在但作者不是调用者 → `model.ErrCommentNotOwner`（A0303）；
* 存在、归属无误但仍没删掉 → 并发删除，按「已经不在了」回 `ErrCommentNotFound`。

### 3. 后台评论列表使用固定视图 `AdminCommentView`

`AdminCommentView` 只含 id、user_id、展示用 user（uid/username/nickname/avatar，**不含 email**）、
content、comment_key、chapter_url、chapter_name、book_name、created_at；分页回显（page/page_size）
不再塞进数据里。后台不再序列化实体：实体加字段不该自动成为对外契约，`email` 这类敏感字段
也不该因为一次 `Preload` 就出现在评论列表里。

### 4. 后台通过 `CommentModule` 接口消费评论能力

`internal/admin.Handler` 的评论依赖从 `CommentStore`（仓储形状）换成
`CommentModule`：`Count` / `CountByUserID` / `ListForAdmin` / `Delete` / `MigrateKeys`。
后台因此不再接触 `model.Comment` 实体与分页钳制；过滤条件仍以 `model.CommentQuery`
交给模块（字段映射留在 handler），治理删只是 `Delete(id, CommentScopeOfAdmin())`，
与作者删共享同一条语义。

后台仍是独立 Gin 引擎、独立 JWT、默认只监听 `127.0.0.1`（ADR-0010 不变）；
本 ADR 改变的是「后台怎么调评论模块」，不是「谁可以访问后台」。

## 后果

* 作者删与治理删只差一个 scope，新增第三种删除方必须构造新的 scope，规则不会被复制。
* 并发删除返回「不存在」而不是假成功；客户端重试幂等。
* 后台评论列表失去 `page`/`page_size` 回显与 `email`；后台前端只读 `list`/`total`，不依赖这两项。
* 治理删仍不做归属校验——这是后台的职责所在，不是遗漏。

测试：`service/comment_test.go`（scope 越权、治理删、固定视图）、
`repository/comment_test.go`（`DeleteScoped` 的条件与 `RowsAffected`）、
`internal/admin/handler_test.go`（后台视图不含 email、治理删成功/不存在）。

锚点：`backend/model/comment.go` 的 `CommentScope`；`backend/service/comment.go` 的 `Delete`；`backend/model/comment.go` 的 `AdminCommentView`。
