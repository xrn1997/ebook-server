package model

import (
	"time"

	"gorm.io/gorm"
)

// Comment 评论实体（GORM 模型）。
//
// 每条评论属于一个用户，支持软删除。
type Comment struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	UserID    uint           `json:"user_id" gorm:"index;not null"` // 关联 users.uid
	User      User           `json:"user" gorm:"foreignKey:UserID"` // 所属用户（预加载）
	Content   string         `json:"content" gorm:"type:text;not null"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Comment) TableName() string {
	return "comments"
}

// CreateCommentRequest 创建评论请求
type CreateCommentRequest struct {
	Content string `json:"content" binding:"required,min=1,max=1000"`
}

// CommentListResponse 评论列表响应
type CommentListResponse struct {
	Items    []Comment `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
