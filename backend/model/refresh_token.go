package model

import "time"

// RefreshToken 刷新令牌记录
//
// 不透明随机串（32 字节 hex）仅以 SHA-256 哈希形式落库，原始串不下发两次，
// 刷新时通过 Rotation 使旧 token 作废。
type RefreshToken struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TokenHash string    `json:"-" gorm:"uniqueIndex;size:64;not null"`
	UserID    uint      `json:"user_id" gorm:"index;not null"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名
func (RefreshToken) TableName() string {
	return "refresh_tokens"
}
