package repository

import (
	"ebook-server/model"
	"ebook-server/pkg/database"
)

type CommentRepository struct{}

func NewCommentRepository() *CommentRepository {
	return &CommentRepository{}
}

// Create 创建评论
func (r *CommentRepository) Create(comment *model.Comment) error {
	return database.GetDB().Create(comment).Error
}

// FindByID 根据 ID 查找评论
func (r *CommentRepository) FindByID(id uint) (*model.Comment, error) {
	var comment model.Comment
	err := database.GetDB().Preload("User").First(&comment, id).Error
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// FindByUserID 根据用户 ID 查找评论
func (r *CommentRepository) FindByUserID(userID uint, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment
	var total int64

	query := database.GetDB().Model(&model.Comment{}).Where("user_id = ?", userID)

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

// FindAll 查找所有评论（分页）
func (r *CommentRepository) FindAll(page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment
	var total int64

	query := database.GetDB().Model(&model.Comment{})

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
	return database.GetDB().Delete(&model.Comment{}, id).Error
}

// CanDelete 检查是否有权限删除
func (r *CommentRepository) CanDelete(commentID, userID uint) (bool, error) {
	var comment model.Comment
	err := database.GetDB().First(&comment, commentID).Error
	if err != nil {
		return false, err
	}
	return comment.UserID == userID, nil
}
