package model

import (
	"time"

	"gorm.io/gorm"
)

// Comment 评论实体（GORM 模型）。
//
// 每条评论属于一个用户，支持软删除。
// 章节归属字段（ChapterURL/ChapterName/BookName）为**冗余快照**：后端不消费
// 书源数据，原样存储客户端提交的值，仅作评论聚合键与展示（ADR-0011）。
// ChapterURL 为空串 = 书籍级评论（不归属任何章节，见 CONTEXT.md「书籍级评论」）。
type Comment struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	UserID      uint           `json:"user_id" gorm:"index;not null"` // 关联 users.uid
	User        User           `json:"user" gorm:"foreignKey:UserID"` // 所属用户（预加载）
	Content     string         `json:"content" gorm:"type:text;not null"`
	ChapterURL  string         `json:"chapter_url" gorm:"type:text;index"` // 书源章节 URL（空=书籍级评论，见 CONTEXT.md），聚合键需索引
	ChapterName string         `json:"chapter_name" gorm:"size:200"`       // 章节名快照
	BookName    string         `json:"book_name" gorm:"size:200"`          // 书名快照
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Comment) TableName() string {
	return "comments"
}

// CreateCommentRequest 创建评论请求
//
// 章节字段全部可选：不传/传空串时行为退化为书籍级评论（兼容既有客户端与历史数据）。
// chapter_url 刻意**不校验 URL 格式**（ADR-0011）：后端无书源数据，无法判断章节
// 是否存在，且第三方书源 URL 形态各异，仅做长度限制防脏数据。
type CreateCommentRequest struct {
	Content     string `json:"content" binding:"required,min=1,max=1000"`
	ChapterURL  string `json:"chapter_url" binding:"omitempty,max=2048"`
	ChapterName string `json:"chapter_name" binding:"omitempty,max=200"`
	BookName    string `json:"book_name" binding:"omitempty,max=200"`
}

// MigrateCommentKeyRequest 迁移评论聚合键请求（合并书籍修键场景）。
//
// old_key/new_key 对应 comment 的 chapter_url 字段；只迁移当前用户名下的评论。
type MigrateCommentKeyRequest struct {
	OldKey string `json:"old_key" binding:"required"`
	NewKey string `json:"new_key" binding:"required"`
}

// MigrateCommentKeyResponse 迁移评论聚合键响应。
type MigrateCommentKeyResponse struct {
	MigratedCount int64 `json:"migrated_count"`
}

// CommentUserView 评论内嵌的用户视图（ADR-0011 契约：仅 uid/username/nickname/avatar）。
//
// 与 User 实体分离：不序列化 email 等账号字段，避免评论列表泄露用户隐私。
type CommentUserView struct {
	UID      uint   `json:"uid"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// CommentResponse 评论响应视图。
//
// 与实体分离的独立契约结构（ADR-0011）：user 只含四个展示字段；
// add_time 固定 Asia/Shanghai 时区 + "yyyy-MM-dd HH:mm:ss" 格式，不依赖服务器时区。
type CommentResponse struct {
	ID          uint            `json:"id"`
	User        CommentUserView `json:"user"`
	Content     string          `json:"content"`
	ChapterURL  string          `json:"chapter_url"`
	ChapterName string          `json:"chapter_name"`
	BookName    string          `json:"book_name"`
	AddTime     string          `json:"add_time"`
}

// NewCommentResponse 把评论实体转换为响应视图。
func NewCommentResponse(c *Comment) CommentResponse {
	return CommentResponse{
		ID:          c.ID,
		User:        newCommentUserView(&c.User),
		Content:     c.Content,
		ChapterURL:  c.ChapterURL,
		ChapterName: c.ChapterName,
		BookName:    c.BookName,
		AddTime:     FormatShanghaiTime(c.CreatedAt),
	}
}

// newCommentUserView 从 User 实体提取评论视图（空用户回零值）。
func newCommentUserView(u *User) CommentUserView {
	if u == nil {
		return CommentUserView{}
	}
	return CommentUserView{
		UID:      u.UID,
		Username: u.Username,
		Nickname: u.Nickname,
		Avatar:   u.Avatar,
	}
}

// CommentListResponse 评论列表响应
type CommentListResponse struct {
	Items    []CommentResponse `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}
