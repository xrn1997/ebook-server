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
func (s *CommentService) Create(userID uint, req *model.CreateCommentRequest) (*model.CommentResponse, error) {
	comment := &model.Comment{
		UserID:      userID,
		Content:     req.Content,
		ChapterURL:  req.ChapterURL,
		ChapterName: req.ChapterName,
		BookName:    req.BookName,
	}

	if err := s.comments.Create(comment); err != nil {
		return nil, err
	}

	// 重新查询以加载关联的用户信息
	created, err := s.comments.FindByID(comment.ID)
	if err != nil {
		return nil, err
	}
	resp := model.NewCommentResponse(created)
	return &resp, nil
}

// GetByBook 按书名获取评论列表（聚合某书全部章节评论，ADR-0011）。
func (s *CommentService) GetByBook(bookName string, page, pageSize int) (*model.CommentListResponse, error) {
	page, pageSize = normalizePage(page, pageSize)

	comments, total, err := s.comments.FindByBook(bookName, page, pageSize)
	if err != nil {
		return nil, err
	}

	return toListResponse(comments, total, page, pageSize), nil
}

// GetByUserID 根据用户 ID 获取评论列表
func (s *CommentService) GetByUserID(userID uint, page, pageSize int) (*model.CommentListResponse, error) {
	page, pageSize = normalizePage(page, pageSize)

	comments, total, err := s.comments.FindByUserID(userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	return toListResponse(comments, total, page, pageSize), nil
}

// GetByChapter 按章节聚合键获取评论列表（ADR-0011）。
//
// chapterURL 必填（由调用方保证非空），bookName 可选二次过滤。
func (s *CommentService) GetByChapter(chapterURL, bookName string, page, pageSize int) (*model.CommentListResponse, error) {
	page, pageSize = normalizePage(page, pageSize)

	comments, total, err := s.comments.FindByChapter(chapterURL, bookName, page, pageSize)
	if err != nil {
		return nil, err
	}

	return toListResponse(comments, total, page, pageSize), nil
}

// GetAll 获取所有评论列表
func (s *CommentService) GetAll(page, pageSize int) (*model.CommentListResponse, error) {
	page, pageSize = normalizePage(page, pageSize)

	comments, total, err := s.comments.FindAll(page, pageSize)
	if err != nil {
		return nil, err
	}

	return toListResponse(comments, total, page, pageSize), nil
}

// toListResponse 把实体列表转换为响应视图列表（ADR-0011 契约）。
func toListResponse(comments []model.Comment, total int64, page, pageSize int) *model.CommentListResponse {
	items := make([]model.CommentResponse, 0, len(comments))
	for i := range comments {
		items = append(items, model.NewCommentResponse(&comments[i]))
	}
	return &model.CommentListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}
}

// Delete 删除评论（仅本人可删，ADR-0011）。
//
// 非本人返回 model.ErrCommentNotOwner（映射 A0303）——与通用 ErrNoPermission
// 分离，客户端可据此给出「无权删除他人评论」的明确提示。
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
		return model.ErrCommentNotOwner
	}

	return s.comments.Delete(commentID)
}
