// Package model 定义数据模型，包含 GORM 实体、请求/响应结构体与业务错误。
package model

import (
	"time"

	"gorm.io/gorm"
)

// User 用户实体（GORM 模型）。
//
// Email 为登录主标识（唯一索引），UID 为主键，Username 为展示用名（非唯一）。
type User struct {
	UID           uint           `json:"uid" gorm:"primaryKey;autoIncrement"`        // 主键，自增
	Email         string         `json:"email" gorm:"uniqueIndex;size:100;not null"` // 登录主标识，唯一
	Password      string         `json:"-" gorm:"size:255;not null"`                 // bcrypt 哈希，不序列化到 JSON
	Username      string         `json:"username" gorm:"size:50;not null"`           // 展示用名，注册时自动生成，可后改
	Nickname      string         `json:"nickname" gorm:"size:50"`                    // 用户昵称
	Avatar        string         `json:"avatar" gorm:"size:255"`                     // 头像 URL
	LoginAttempts int            `json:"-" gorm:"default:0"`                         // 连续登录失败次数（ADR-0002）
	LockedUntil   *time.Time     `json:"-"`                                          // 登录锁定截止时间（nil 表示未锁定）
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}

// RegisterRequest 注册请求（邮箱+验证码+密码，激活建号）
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Code     string `json:"code" binding:"required,len=6"`
	Password string `json:"password" binding:"required,min=6,max=50"`
}

// LoginRequest 登录请求（邮箱+密码）
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// UpdateUserRequest 更新用户请求
//
// Email 保留在结构体中仅供「显式拒绝」之用（ADR-0004）：登录主标识不可变更，
// 若客户端传了与当前值不同的 email，service 会返回 model.ErrEmailImmutable。
// 若移除该字段，Gin 会静默丢弃未知字段，客户端会误以为修改成功。
type UpdateUserRequest struct {
	Username string `json:"username" binding:"omitempty,min=3,max=50"`
	Nickname string `json:"nickname" binding:"omitempty,min=1,max=50"`
	Email    string `json:"email" binding:"omitempty,email"`
	Avatar   string `json:"avatar" binding:"omitempty,url"`
}

// ModifyPwdRequest 已登录修改密码请求
type ModifyPwdRequest struct {
	OldPassword string `json:"old_password" binding:"required,min=6,max=50"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=50"`
}

// SendCodeRequest 按邮箱发送验证码请求（注册 / 找回密码共用）
type SendCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ResetPasswordRequest 验证码重置密码请求（纯邮箱）
type ResetPasswordRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Code        string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=50"`
}

// TokenPair 双 token 载荷
type TokenPair struct {
	AccessToken  string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	User         User   `json:"user"`
}

// TokenPayload 纯凭证载荷（刷新端点用，不含用户资料）
type TokenPayload struct {
	AccessToken  string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshRequest 刷新 token 请求
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest 登出请求
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// DeleteAccountRequest 注销账号请求（邮箱验证码确认，ADR-0005）
type DeleteAccountRequest struct {
	Code string `json:"code" binding:"required,len=6"`
}

// UserInfo 导出中的用户资料，不含任何凭据
type UserInfo struct {
	UID       uint      `json:"uid"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Avatar    string    `json:"avatar"`
	CreatedAt time.Time `json:"created_at"`
}

// ExportedComment 导出中的评论条目。使用独立结构而非 Comment，
// 避免序列化出空的关联用户对象（encoding/json 对结构体不支持 omitempty）。
type ExportedComment struct {
	ID        uint      `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// UserDataExport 用户自有数据的完整副本（ADR-0005）。
//
// 用于注销时当场交付，也用于 GET /api/users/me/data。
// 刻意不含：密码哈希、操作日志（其 RequestBody 可能记录明文密码）、任何内部凭据。
type UserDataExport struct {
	ExportedAt time.Time         `json:"exported_at"`
	User       UserInfo          `json:"user"`
	Comments   []ExportedComment `json:"comments"`
}
