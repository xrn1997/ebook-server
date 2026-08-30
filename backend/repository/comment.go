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

// FindByChapter 按章节聚合键查找评论（分页）。
//
// chapterURL 精确匹配（聚合键，ADR-0011）；bookName 可选二次过滤；
// 排序与全局列表一致（created_at DESC）。bookName 为空时不参与过滤。
func (r *CommentRepository) FindByChapter(chapterURL, bookName string, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment
	var total int64

	query := r.db.Model(&model.Comment{}).Where("chapter_url = ?", chapterURL)
	if bookName != "" {
		query = query.Where("book_name = ?", bookName)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Preload("User").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&comments).Error; err != nil {
		return nil, 0, err
	}

	return comments, total, nil
}

// FindByBook 按书名查找评论（分页），聚合该书的全部章节评论。
//
// book_name 精确匹配；排序与全局列表一致（created_at DESC）。
func (r *CommentRepository) FindByBook(bookName string, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment
	var total int64

	query := r.db.Model(&model.Comment{}).Where("book_name = ?", bookName)

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Preload("User").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&comments).Error; err != nil {
		return nil, 0, err
	}

	return comments, total, nil
}

// FindByUserID 根据用户 ID 查找评论
func (r *CommentRepository) FindByUserID(userID uint, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment
	var total int64

	query := r.db.Model(&model.Comment{}).Where("user_id = ?", userID)

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Preload("User").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&comments).Error; err != nil {
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

// FindAll 查找所有评论（分页）
func (r *CommentRepository) FindAll(page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment
	var total int64

	query := r.db.Model(&model.Comment{})

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Preload("User").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&comments).Error; err != nil {
		return nil, 0, err
	}

	return comments, total, nil
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
