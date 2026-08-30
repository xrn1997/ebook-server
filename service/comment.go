package service

import (
	"ebook-server/model"
)

// CommentService 评论业务服务。
type CommentService struct {
	comments CommentStore
}

// NewCommentService 创建评论服务实例。
func NewCommentService(comments CommentStore) *CommentService {
	return &CommentService{
		comments: comments,
	}
}

// normalizePage 归一化分页参数：页码最小为 1，每页数量缺省 10、上限 100。
//
// 分页语义是全站列表接口的统一约定，收敛在一处，避免各服务各写一份钳制。
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return page, pageSize
}

// Create 创建评论
func (s *CommentService) Create(userID uint, req *model.CreateCommentRequest) (*model.Comment, error) {
	comment := &model.Comment{
		UserID:  userID,
		Content: req.Content,
	}

	if err := s.comments.Create(comment); err != nil {
		return nil, err
	}

	// 重新查询以加载关联的用户信息
	return s.comments.FindByID(comment.ID)
}

// GetByID 根据 ID 获取评论
func (s *CommentService) GetByID(id uint) (*model.Comment, error) {
	comment, err := s.comments.FindByID(id)
	if err != nil {
		if IsRecordNotFound(err) {
			return nil, model.ErrCommentNotFound
		}
		return nil, err
	}
	return comment, nil
}

// GetByUserID 根据用户 ID 获取评论列表
func (s *CommentService) GetByUserID(userID uint, page, pageSize int) (*model.CommentListResponse, error) {
	page, pageSize = normalizePage(page, pageSize)

	comments, total, err := s.comments.FindByUserID(userID, page, pageSize)
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
	page, pageSize = normalizePage(page, pageSize)

	comments, total, err := s.comments.FindAll(page, pageSize)
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
	canDelete, err := s.comments.CanDelete(commentID, userID)
	if err != nil {
		if IsRecordNotFound(err) {
			return model.ErrCommentNotFound
		}
		return err
	}
	if !canDelete {
		return model.ErrNoPermission
	}

	return s.comments.Delete(commentID)
}
