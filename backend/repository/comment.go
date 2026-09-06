package repository

import (
	"ebook-server/model"

	"gorm.io/gorm"
)

// CommentRepository 公开内容（评论）数据访问。满足 service.CommentStore 接口。
type CommentRepository struct {
	db *gorm.DB
}

// NewCommentRepository 创建评论仓库实例。
func NewCommentRepository(db *gorm.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

// Create 创建评论
func (r *CommentRepository) Create(comment *model.Comment) error {
	return r.db.Create(comment).Error
}

// FindByID 根据 ID 查找评论
func (r *CommentRepository) FindByID(id uint) (*model.Comment, error) {
	var comment model.Comment
	err := r.db.Preload("User").First(&comment, id).Error
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// FindByChapterURLs 按章节聚合键查找评论并集（分页）。
//
// 传入单个键即等价于按该键精确匹配，因此不再另设单键方法（两者的查询体只差一个 WHERE 形式）。
// bookName 为空时不参与过滤；排序与全局列表一致（created_at DESC）。
func (r *CommentRepository) FindByChapterURLs(chapterURLs []string, bookName string, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment

	query := r.db.Model(&model.Comment{}).Where("chapter_url IN ?", chapterURLs).Preload("User")
	if bookName != "" {
		query = query.Where("book_name = ?", bookName)
	}

	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &comments)
	if err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

// FindByBook 按书名查找评论（分页），聚合该书的全部章节评论。
//
// book_name 精确匹配；排序与全局列表一致（created_at DESC）。
func (r *CommentRepository) FindByBook(bookName string, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment

	query := r.db.Model(&model.Comment{}).Where("book_name = ?", bookName).Preload("User")
	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &comments)
	if err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

// FindByUserID 根据用户 ID 查找评论
func (r *CommentRepository) FindByUserID(userID uint, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment

	query := r.db.Model(&model.Comment{}).Where("user_id = ?", userID).Preload("User")
	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &comments)
	if err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

// FindAllByUserID 获取某用户的全部评论（不分页，供数据导出使用）。
//
// 刻意不 Preload("User")——导出用独立的轻量结构，不需要关联用户对象。
func (r *CommentRepository) FindAllByUserID(userID uint) ([]model.Comment, error) {
	var comments []model.Comment
	if err := r.db.
		Where("user_id = ?", userID).
		Order("created_at ASC").
		Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

// Count 统计未删除的评论总数（后台概览用）。
func (r *CommentRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&model.Comment{}).Count(&count).Error
	return count, err
}

// FindAll 查找所有评论（分页）
func (r *CommentRepository) FindAll(page, pageSize int) ([]model.Comment, int64, error) {
	return r.Search(model.CommentQuery{}, page, pageSize)
}

// Search 按筛选条件分页查评论（后台「评论搜索」）。
//
// 零值条件不参与过滤，空条件即等价于 FindAll，因此不再单列一个全量方法。
// keyword 走 content 模糊匹配，LIKE 通配符已转义。
func (r *CommentRepository) Search(q model.CommentQuery, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment

	query := r.db.Model(&model.Comment{}).Preload("User")
	if q.Keyword != "" {
		pattern := likePattern(q.Keyword)
		query = query.Where("content LIKE ? ESCAPE '\\'", pattern)
	}
	if q.BookName != "" {
		query = query.Where("book_name = ?", q.BookName)
	}

	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &comments)
	if err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

// CountByUserID 统计某用户未删除的评论数（后台用户详情用）。
func (r *CommentRepository) CountByUserID(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.Comment{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// Delete 软删除评论
func (r *CommentRepository) Delete(id uint) error {
	return r.db.Delete(&model.Comment{}, id).Error
}

// CanDelete 检查是否有权限删除
func (r *CommentRepository) CanDelete(commentID, userID uint) (bool, error) {
	var comment model.Comment
	err := r.db.First(&comment, commentID).Error
	if err != nil {
		return false, err
	}
	return comment.UserID == userID, nil
}

// MigrateKey 批量迁移某用户在旧聚合键下的评论到新聚合键。
//
// 只更新 chapter_url（聚合键）；chapter_name/book_name 是展示快照不随迁移更新。
// GORM 自动附加 deleted_at IS NULL 条件，软删除记录不受影响。
func (r *CommentRepository) MigrateKey(userID uint, oldKey, newKey string) (int64, error) {
	result := r.db.Model(&model.Comment{}).
		Where("user_id = ? AND chapter_url = ?", userID, oldKey).
		Update("chapter_url", newKey)
	return result.RowsAffected, result.Error
}
