// Package service 实现业务逻辑层，编排认证、账号、评论等业务流程。
//
// 本文件定义 service 层对数据访问与外部能力的接口（consumer-defined，ADR-0007）：
// 接口只列出 service 真正调用的方法，宽度由消费方决定而不是实现方。
// 每个请求作用域的方法第一个参数是 context.Context（ADR-0022）；纯计算与外部
// 没有取消路径的 helper（Mailer、AvatarCleaner）保持无 ctx。
package service

import (
	"context"
	"time"

	"ebook-server/model"
)

// UserStore 认证与注销所需的账号读写。登录主标识（email）的唯一性约束由实现保证
// （ADR-0002/0004）；创建账号走 CredentialStore（密码与建号是同一个意图）。
type UserStore interface {
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByUID(ctx context.Context, uid uint) (*model.User, error)
	Update(ctx context.Context, user *model.User) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}

// ProfileStore 账号资料的读写能力（consumer-defined，ADR-0007）。
//
// 只更新本次提交的资料列：整行 Save 会把并发的登录计数/密码写回旧值，
// 而把读到的三个字段一并写回还会让并发的资料更新互相覆盖。
type ProfileStore interface {
	FindByUID(ctx context.Context, uid uint) (*model.User, error)
	UpdateProfile(ctx context.Context, uid uint, update model.ProfileUpdate) error
	UpdateAvatar(ctx context.Context, uid uint, avatar string) error
	// CountByAvatar 统计头像仍指向该文件的账号数（含当前账号）。
	// 比较 URL 的文件名而不是整串 URL：同一文件可能以不同 host 写法入库，
	// 按整串比较会漏掉引用，进而误删别人的文件。
	CountByAvatar(ctx context.Context, avatar string) (int64, error)
}

// AvatarProfile 账号注销需要的窄能力：清空头像列并删除文件。
//
// 由 *ProfileService 满足；AccountService 只声明自己用到的那一方法。
type AvatarProfile interface {
	ClearAvatar(ctx context.Context, uid uint) error
}

// AvatarCleaner 头像文件清理（consumer-defined，ADR-0007）。
//
// 由 pkg/upload.Store 满足：只删本服务 uploads/avatar 目录内的文件，
// 外部 URL 不做任何操作；删除失败返回 error 由调用方记录。
// os.Remove 没有取消路径，因此保持无 ctx（ADR-0022 的边界）。
type AvatarCleaner interface {
	DeleteAvatar(url string) error
}

// CredentialStore 密码与登录失败计数的写路径（consumer-defined，ADR-0007）。
//
// 失败记账只有一条语句：递增、按阈值上锁、上锁清零都在 RecordFailedLogin 里完成，
// 由数据库给出「现在是否被锁」；不再整行 Save，也不回到 Go 里比大小。
type CredentialStore interface {
	Create(ctx context.Context, user *model.User) error
	UpdatePasswordHash(ctx context.Context, uid uint, hash string) error
	RecordFailedLogin(ctx context.Context, uid uint, maxAttempts int, until time.Time) (bool, error)
	ClearLoginFailures(ctx context.Context, uid uint) error
}

// TokenStore refresh token 存储。签发即落库、轮换即作废（ADR-0003）。
type TokenStore interface {
	Create(ctx context.Context, token *model.RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*model.RefreshToken, error)
	// DeleteByID 条件删除并返回是否真的删掉一行：轮换的「单次消费」判据。
	// 两个并发刷新都查到同一令牌时，只有先删到行的那一个可以签发新令牌。
	DeleteByID(ctx context.Context, id uint) (bool, error)
	DeleteByUserID(ctx context.Context, userID uint) error
}

// CommentStore 公开内容（评论）存储。见 CONTEXT.md「公开内容」。
//
// 读路径只有一个 List(filter)：M2 键 / 已废弃的 chapter_url / 本人 / 书名 / 全局
// 五种形态差的是 WHERE，不是五个方法——优先级与边界归评论模块，键形态归实现。
type CommentStore interface {
	Create(ctx context.Context, comment *model.Comment) error
	FindByID(ctx context.Context, id uint) (*model.Comment, error)
	List(ctx context.Context, filter model.CommentFilter) ([]model.Comment, int64, error)
	FindAllByUserID(ctx context.Context, userID uint) ([]model.Comment, error)
	// MigrateKey 按归属范围批量改键（作用列 comment_key），返回受影响行数。
	// 归属由 scope 表达：限定用户 = 本人迁移，不限用户 = 后台全局改键。
	MigrateKey(ctx context.Context, scope model.CommentKeyScope, oldKey, newKey string) (int64, error)
	// DeleteScoped 条件软删除：作者 scope 同时匹配 user_id，治理 scope 只匹配 id。
	// 返回是否真的删掉了一行（RowsAffected），不再「查到就算删成功」。
	DeleteScoped(ctx context.Context, id uint, scope model.CommentScope) (bool, error)
	// Search 后台评论搜索（内容关键字 + 书名）；零值条件即全量。
	Search(ctx context.Context, query model.CommentQuery, page, pageSize int) ([]model.Comment, int64, error)
	Count(ctx context.Context) (int64, error)
	CountByUserID(ctx context.Context, userID uint) (int64, error)
}

// LogStore 操作日志存储。用户可读、不可写、不可删——见 CONTEXT.md「操作日志」。
//
// 只有 FindByUserID：客户端表面没有「读全站日志」这一能力（跨用户的审计读取
// 只在后台 `/admin/api/logs`，那条路径另有自己的筛选）。
type LogStore interface {
	Create(ctx context.Context, log *model.OperationLog) error
	FindByUserID(ctx context.Context, userID uint, page, pageSize int) ([]model.OperationLog, int64, error)
}

// Mailer 验证码邮件发送。
//
// adapter 有三个：SMTP 真实发送、写日志（本地联调降级）、记录型 fake（测试）。
// 生产环境选哪个由 main.go 按配置装配；本接口刻意不放在 pkg/mail——
// 它是 service 的依赖视角，不是邮件库的能力清单（ADR-0007）。
// net/smtp 没有 context 支持，因此这里不强行加 ctx（ADR-0022）。
type Mailer interface {
	SendCode(to, codeVal string) error
}
