// Package service 实现业务逻辑层，编排认证、账号、评论等业务流程。
//
// 本文件定义 service 层对数据访问与外部能力的接口（consumer-defined，ADR-0007）：
// 接口只列出 service 真正调用的方法，宽度由消费方决定而不是实现方。
// 每个接口在生产环境由 repository 包的 gorm adapter 满足，
// 测试环境由同一实现 + 独立 SQLite :memory: 实例满足（pkg/testdb）。
package service

import (
	"time"

	"ebook-server/model"
	"ebook-server/pkg/ratelimit"
)

// UserStore 账号存储。登录主标识（email）的唯一性约束由实现保证（ADR-0002/0004）。
type UserStore interface {
	Create(user *model.User) error
	FindByEmail(email string) (*model.User, error)
	FindByUID(uid uint) (*model.User, error)
	Update(user *model.User) error
	ExistsByEmail(email string) (bool, error)
}

// TokenStore refresh token 存储。签发即落库、轮换即作废（ADR-0003）。
type TokenStore interface {
	Create(token *model.RefreshToken) error
	FindByHash(hash string) (*model.RefreshToken, error)
	DeleteByID(id uint) error
	DeleteByUserID(userID uint) error
}

// CommentStore 公开内容（评论）存储。见 CONTEXT.md「公开内容」。
type CommentStore interface {
	Create(comment *model.Comment) error
	FindByID(id uint) (*model.Comment, error)
	FindAll(page, pageSize int) ([]model.Comment, int64, error)
	FindByUserID(userID uint, page, pageSize int) ([]model.Comment, int64, error)
	FindAllByUserID(userID uint) ([]model.Comment, error)
	Delete(id uint) error
	CanDelete(commentID, userID uint) (bool, error)
}

// LogStore 操作日志存储。用户可读、不可写、不可删——见 CONTEXT.md「操作日志」。
type LogStore interface {
	Create(log *model.OperationLog) error
	FindAll(page, pageSize int) ([]model.OperationLog, int64, error)
	FindByUserID(userID uint, page, pageSize int) ([]model.OperationLog, int64, error)
}

// Mailer 验证码邮件发送。
//
// adapter 有三个：SMTP 真实发送、写日志（本地联调降级）、记录型 fake（测试）。
// 生产环境选哪个由 main.go 按配置装配；本接口刻意不放在 pkg/mail——
// 它是 service 的依赖视角，不是邮件库的能力清单（ADR-0007）。
type Mailer interface {
	SendCode(to, codeVal string) error
}

// newSendCodeLimiters 创建一对新的发码限流器（ADR-0002：每分钟 1 次、每小时 5 次）。
//
// 随 service 实例创建而非包级共享，使每个实例（含测试实例）拥有独立配额状态；
// 限流键自带流程前缀（见 sendCodeKey），多个实例并存不会互相放大配额。
func newSendCodeLimiters() (minute, hour *ratelimit.Limiter) {
	return ratelimit.New(1, time.Minute), ratelimit.New(5, time.Hour)
}

// allowSendCode 发码频率限流：两道窗口任一超限即拒绝。
func allowSendCode(minute, hour *ratelimit.Limiter, key string) error {
	if !minute.Allow(key) || !hour.Allow(key) {
		return model.ErrAttemptTooMany
	}
	return nil
}
