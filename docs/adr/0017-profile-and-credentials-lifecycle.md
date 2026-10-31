# 账号资料与凭据的生命周期收进模块

Status: accepted

账号资料和凭据此前散在两个模块与若干调用点里：

* `UserService.Update` 读整行、改字段、`Save` 整行。并发登录失败计数或改密会被这次
  Save 写回旧值——一次改昵称就能把刚锁定的账号解锁、把新密码改回旧的。
* bcrypt 的哈希/比对、失败计数、锁定参数写在 `AuthService`，改密与重置密码各自复制
  一次「删 token → 写哈希」的顺序。
* 头像有两个事实（`users.avatar` 的 URL 与 `uploads/avatar` 下的文件），更新资料与注销
  各处理一半；注销清空头像列时文件留在磁盘上。

## 决策

### 1. `ProfileService` 是账号资料的唯一写入口

* `ProfileStore` 只暴露列级写：`UpdateProfile(uid, ProfileUpdate)`（`ProfileUpdate` 用
  `*string` 表达「本次提交了哪几列」）与 `UpdateAvatar(uid, avatar)`。**资料写入路径上的**
  整行 Save 已删除——改资料不再触碰密码、登录计数与锁定时间，也不会把未提交的展示列
  覆盖回旧值。
  `UserRepository.Update`（整行 Save）刻意保留，但只剩账号匿名化这一个生产调用方
  （`AccountService.Delete`）：那条路径本就要把这些列一并抹平，不存在「写回旧值」
  的语义错误；不包事务的理由见 ADR-0005。新增整行写路径前先重读本节。
* 部分更新语义不变：空串表示不修改；`email` 不可变（ADR-0004），传了不同的值回
  `ErrEmailImmutable`。
* 头像文件生命周期收进同一模块：换头像时先写新 URL、再尽力删除旧文件；删除失败只
  `zap.Warn`，旧文件成孤儿由运维清理，不回滚已生效的头像。
* 删除文件前先统计**是否还有账号的头像指向同一个文件**，有引用就保留。比较的是 URL 的
  文件名而不是整串 URL：头像 URL 的来源随上传时的请求 Host 或 `server.public_base_url`
  而变，同一文件可能以
  `localhost` 与内网 IP 两种写法入库；按整串比较会漏掉换 host 的引用。文件名比较还忽略
  大小写——Windows/macOS 的文件系统不区分大小写，按字面比较会被大小写变体绕过。
  计数**包含当前账号自己**——UpdateProfile 先写新 URL 再查询，新旧只是 host 不同时
  这一票负责保文件（排除自己会把用户正在用的头像删掉）。头像 URL 是公开的
  （评论列表会带出 `user.avatar`），而资料更新接受客户端提供的 URL；不查引用就删文件，
  等于允许任何人把别人的头像路径改个 host/大小写填进自己的资料、再换头像/注销删掉对方的文件。
* `AccountService` 通过窄接口 `AvatarProfile.ClearAvatar(uid)` 在匿名化前清空头像列并删文件；
  它不直接操作 `ProfileStore` 或上传目录。

### 2. `Credentials` 是密码与登录失败策略的唯一实现

* `CreateWithPassword`：唯一的「哈希 + 建号」入口。
* `CheckPassword`：只回答真/假，业务错误由认证模块决定。
* `SetPassword`：**先撤销该用户全部 refresh token，再写新哈希**（fail-closed）。两种失败
  都不会留下「新密码生效、旧 refresh token 仍活着」的状态；最坏情况是用户被登出、
  旧密码仍可用，可重新登录重试。
* `RecordFailedLogin`：原子 `UPDATE login_attempts = login_attempts + 1`，达到 5 次写入
  `locked_until` **并同时清零计数**——锁定期过后仍有完整的 5 次机会，而不是「再失败一次
  就立刻重锁」。登录成功后无条件清零（避免读到过期计数）。
* 失败计数用单条原子 UPDATE，而不是「读整行 → 加一 → Save」；这是与资料更新并发时
  互不覆盖的前提。

### 3. 不引入跨仓储事务

头像文件与数据库列、refresh token 与密码哈希，各自都是「两个存储、一个意图」。为它们
引入 TxRunner/补偿事务的复杂度，换来的只是把可自愈的瞬态（重新登录、重新上传）变成
分布式事务问题。选择是：先做不可逆性更低的一步、失败时保持旧值可用、把不可回滚的
清理动作（删文件）降级为告警。账号注销的整体顺序与「不包事务」的理由见 ADR-0005。

## 后果

* 改昵称/头像不会覆盖并发的密码、失败计数与锁定状态，也不会覆盖另一个请求刚写入的
  其他资料列；测试 `TestCredentials_RecordFailedLogin_DoesNotClobberProfile` 与
  `TestUserRepository_UpdateProfile_OnlyWritesProvidedColumns` 钉住这两条。
* 注销账号会在没有账号引用该文件（按文件名判定，host 写法不同也算）时真正删除本服务
  托管的头像文件（外部 URL 仍是 no-op；共享文件保留）。
* 密码策略（bcrypt cost、锁定阈值、锁定时长）只在 `credentials.go` 一处；`AuthService`
  只剩「查到账号 → 校验 → 记录成功/失败 → 签发」的编排。
* 删除旧头像失败不回滚资料更新，日志里能查到孤儿文件。

测试：`service/profile_test.go`（资料写路径从库里读回来断言，不判返回的内存对象；
同一文件的 host/大小写写法变化与「外部 host 伪造路径」都不得删掉别的账号仍在用的文件）、
`service/credentials_test.go`、`service/account_test.go`（注销删头像且导出仍含旧 URL）、
`repository/user_test.go`（每条列级写各只碰自己的列：`UpdateProfile` 不越凭据且只写
本次提交的列、失败计数原子递增且锁定清零、改密不越资料、`CountByAvatar` 按文件名统计且包含自己）。

锚点：`backend/service/profile.go`；`backend/service/credentials.go`；`backend/service/profile_test.go`；`backend/service/credentials_test.go`。
