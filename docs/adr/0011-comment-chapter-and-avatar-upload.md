# 评论章节归属与头像文件上传

为公开评论增加「章节归属」能力（按书源章节 URL 组织评论），并为头像引入
multipart 文件上传 + 本地磁盘存储。两项均来自 android-ebook 客户端的需求单
（`docs/backend-api-change-request.md`，v1.0）。

## 现状

客户端阅读器评论区按**书源章节 URL** 组织评论，但后端评论模型只有
`{content, user, created_at}`：

* `POST /api/comments` 请求体仅 `content`——评论无法关联书籍/章节，发表即丢失归属；
* `GET /api/comments` 无章节过滤能力，无法「查某章节的评论」；
* 响应直接序列化 `model.Comment`——user 对象含 `email` 等账号字段（泄露隐私），
  时间字段为 RFC3339，与客户端契约（`add_time` yyyy-MM-dd HH:mm:ss）不符；
* 头像只有 URL 字符串字段，无文件上传端点、无静态文件服务。

## 决策

### 1. 章节字段是「冗余快照」，不是外键

`Comment` 增加 `chapter_url / chapter_name / book_name` 三字段，**原样存储客户端提交
的值**，后端不消费、不校验格式（仅长度限制）：

* 后端没有书源数据，无法判断「章节是否存在」——`A0302`（章节非法/不存在）预留不启用；
* 第三方书源 URL 形态各异，格式校验会误伤真实数据；
* `chapter_url` 是聚合键（精确匹配），`chapter_name/book_name` 是展示快照。

`chapter_url` 为空串 = 书籍级评论，兼容既有客户端与历史数据（新字段全部可选）。

### 2. 评论响应使用独立视图，不序列化实体

新建 `CommentResponse` 视图：

* `user` 只含 `{uid, username, nickname, avatar}` 四字段——不泄露 `email` 等账号字段；
* `add_time` 固定 Asia/Shanghai 时区 + `yyyy-MM-dd HH:mm:ss`（`time.FixedZone`，不依赖
  服务器时区与 tzdata）；
* 列表保持既有包裹结构 `{items, total, page, page_size}`（客户端按此适配）。

### 3. 删除权限专用错误码

删除评论的「本人校验」本已存在（`CanDelete`），本次把通用 `A0403` 替换为评论域
专用码 `A0303`（新增 `ErrCommentNotOwner` sentinel），`A0304` 替代 `A0404` 表示
「评论不存在」。`A0301`（内容为空）/`A0302`（章节非法）为**契约预留码**——需求单
§5.3 明确要求预留 `A03xx` 段，常量保留但本次不启用（`content` 由 binding 兜底、
后端无书源数据不做章节存在性校验），是 API 契约而非死码。
「管理员可删」暂不实现：后台管理面当前只有只读列表，无删除能力，属二期范围。

### 4. 头像上传：本地磁盘 + 拼接请求 Host 的绝对 URL

新增 `POST /api/uploads/avatar`（multipart，字段名 `avatar`，**必须登录**）：

* 校验：扩展名白名单 jpg/jpeg/png/webp + ≤5MB（`MaxBytesReader` 防恶意超大）；
* 命名：`<unix毫秒>_<16字节随机hex>.<ext>`，防碰撞与路径注入；
* 存储：`upload.dir`（默认 `uploads`）下的 `avatar/` 子目录，经 `/uploads/*` 静态公开；
* 返回：`{url: "<scheme>://<请求Host>/uploads/avatar/<name>"}`——自托管场景无固定
  域名，用请求 Host 动态拼接（TLS 有无决定 scheme）；
* 客户端两步提交：先上传拿 URL → 再 `PUT /api/users/me {avatar: url}`。

### 5. 更换头像删除旧文件（业务层编排）

`UserService.Update`（service 层）更新头像成功后，若旧头像 URL 属于本服务
`uploads/avatar/`（`url.Parse` + 前缀 + 纯文件名校验，防穿越），删除旧文件；外部
URL 或删除失败均只告警不阻断（不引入事务复杂度，孤儿文件由运维清理）。

* **编排位置在 service 层**：`UserService` 经 consumer-defined 接口 `AvatarCleaner`
  （由 `pkg/upload.Store` 满足）注入清理能力，与「handler 只做协议转换」的分层
  约定一致；handler 不持有 `*upload.Store`。
* 清理条件：新头像非空且与旧值不同（`req.Avatar != "" && old != new`）。当前更新
  语义下 `avatar` 置空串不会清除旧值（`Update` 只写非空字段），因此不存在
  「置空后旧文件成孤儿」的路径；若未来支持显式清空头像，清理逻辑在同一处扩展。
* `DeleteAvatar` 不校验 URL 的 Host：头像文件名为 128-bit 随机值、删除只作用于
  `avatarDir` 内文件、且触发前提是用户自身的资料更新操作——攻击者无法构造
  「外部域名 + 本服务路径」的组合来删除他人文件（需先让其成为该用户的旧头像）。

## 被拒方案

* **章节字段做外键/关联书源表**：后端没有书源数据源，引入空表与无效约束。
* **校验 chapter_url 格式（http 前缀）**：会拒掉形态各异的第三方书源 URL，收益为负。
* **头像存数据库 BLOB / 外部图床**：BLOB 拖垮 SQLite 单文件；图床依赖第三方可用性，
  不符合自托管定位。
* **返回相对路径由客户端拼 base**：客户端需额外维护 base URL，不如服务端一次拼好。

## 下游影响

* `model.Comment` 增加 3 字段（GORM AutoMigrate 加列，存量数据 `chapter_url` 为空串）；
  `chapter_url` 是聚合键，加索引（`gorm:"type:text;index"`）防全表扫描；
* `CommentStore` 接口新增 `FindByChapter`（chapter_url 精确 + book_name 可选二次过滤）
  与 `FindByBook`（book_name 单独过滤全书）；
* `service.CommentService` 的 Create/列表方法改返回 `CommentResponse` 视图；
  `GetList` 按 `chapter_url` → `book_name` → 全局 三级降级过滤；
* `pkg/errcode` 新增 `A0301`–`A0304`；`ErrCommentNotFound → A0304`、
  `ErrCommentNotOwner → A0303`；
* 新增 `pkg/upload`（存储/校验/删除/URL 拼接）与 `handler.UploadHandler`；
* `config` 新增 `upload.dir`（默认 `uploads`）；
* `main.go` 注册 `/api/uploads/avatar`（需登录）与 `/uploads/*` 静态路由；
* `service.UserService` 新增 `AvatarCleaner` 依赖（consumer-defined，由 upload.Store 满足），
  `Update` 更新后清理旧头像文件；`handler.UserHandler` 不持有上传存储，只做协议转换；
* `CONTEXT.md` 新增「章节评论」「书籍级评论」术语；本 ADR 记录决策。

## 验收

1. `go build ./...`、`go test -count=1 ./...` 全绿（含新增 upload 包与 handler 测试）；
2. 创建评论携带章节字段 → 响应含 `user{uid,username,nickname,avatar}` + `add_time`，
   `chapter_url` 持久化；
3. `GET /api/comments?chapter_url=` 只返回该章节评论（时间倒序），无过滤保持全局；
4. 非本人删除评论返回 `A0303`，本人删除成功，不存在返回 `A0304`；
5. 上传头像返回 `http://<host>/uploads/avatar/<随机名>`，直接访问 200；
6. 更换头像后旧文件被删除（`uploads/avatar/` 仅剩新文件）；
7. 运行时端到端验证通过（见实现记录）。
