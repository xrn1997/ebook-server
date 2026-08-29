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
	UID           uint           `json:"uid" gorm:"primaryKey;autoIncrement"`                       // 主键，自增
	Email         string         `json:"email" gorm:"uniqueIndex;size:100;not null"`                 // 登录主标识，唯一
	Password      string         `json:"-" gorm:"size:255;not null"`                                 // bcrypt 哈希，不序列化到 JSON
	Username      string         `json:"username" gorm:"size:50;not null"`                           // 展示用名，注册时自动生成，可后改
	Nickname      string         `json:"nickname" gorm:"size:50"`                                    // 用户昵称
	Avatar        string         `json:"avatar" gorm:"size:255"`                                     // 头像 URL
	LoginAttempts int            `json:"-" gorm:"default:0"`                                         // 连续登录失败次数（ADR-0002）
	LockedUntil   *time.Time     `json:"-"`                                                          // 登录锁定截止时间（nil 表示未锁定）
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
