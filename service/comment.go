package service

import (
	"ebook-server/model"
	"ebook-server/repository"
)

type CommentService struct {
	commentRepo *repository.CommentRepository
}

func NewCommentService() *CommentService {
	return &CommentService{
		commentRepo: repository.NewCommentRepository(),
	}
}

// Create 创建评论
func (s *CommentService) Create(userID uint, req *model.CreateCommentRequest) (*model.Comment, error) {
	comment := &model.Comment{
		UserID:  userID,
		Content: req.Content,
	}

	if err := s.commentRepo.Create(comment); err != nil {
		return nil, err
	}

	// 重新查询以加载关联的用户信息
	return s.commentRepo.FindByID(comment.ID)
}

// GetByID 根据 ID 获取评论
func (s *CommentService) GetByID(id uint) (*model.Comment, error) {
	comment, err := s.commentRepo.FindByID(id)
	if err != nil {
		if repository.IsRecordNotFound(err) {
			return nil, model.ErrCommentNotFound
		}
		return nil, err
	}
	return comment, nil
}

// GetByUserID 根据用户 ID 获取评论列表
func (s *CommentService) GetByUserID(userID uint, page, pageSize int) (*model.CommentListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	comments, total, err := s.commentRepo.FindByUserID(userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	return &model.CommentListResponse{
		Items:    comments,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetAll 获取所有评论列表
func (s *CommentService) GetAll(page, pageSize int) (*model.CommentListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	comments, total, err := s.commentRepo.FindAll(page, pageSize)
	if err != nil {
		return nil, err
	}

	return &model.CommentListResponse{
		Items:    comments,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// Delete 删除评论
func (s *CommentService) Delete(commentID, userID uint) error {
	// 检查权限
	canDelete, err := s.commentRepo.CanDelete(commentID, userID)
	if err != nil {
		if repository.IsRecordNotFound(err) {
			return model.ErrCommentNotFound
		}
		return err
	}
	if !canDelete {
		return model.ErrNoPermission
	}

	return s.commentRepo.Delete(commentID)
}
