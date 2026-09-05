package handler

import (
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

// CommentHandler 评论 HTTP 处理器。
type CommentHandler struct {
	commentService *service.CommentService
}

// NewCommentHandler 创建评论处理器实例。
func NewCommentHandler(commentService *service.CommentService) *CommentHandler {
	return &CommentHandler{
		commentService: commentService,
	}
}

// Create 创建评论
// @Summary 创建评论
// @Description 创建新评论
// @Tags 评论
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body model.CreateCommentRequest true "评论内容"
// @Success 200 {object} model.Response
// @Router /api/comments [post]
func (h *CommentHandler) Create(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	var req model.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	comment, err := h.commentService.Create(userID, &req)
	if err != nil {
		errcode.Respond(c, err, "创建评论失败")
		return
	}

	errcode.Success(c, comment)
}

// GetList 获取评论列表
// @Summary 获取评论列表
// @Description 获取评论列表，可按章节过滤（chapter_url 支持多个，返回并集；book_name 可单独或配合过滤）
// @Tags 评论
// @Produce json
// @Param chapter_url query []string false "书源章节聚合键（可传多个，返回并集）"
// @Param book_name query string false "书名（配合 chapter_url 二次过滤，或单独过滤全书）"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /api/comments [get]
func (h *CommentHandler) GetList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	chapterURLs := c.QueryArray("chapter_url")
	// 过滤空串（?chapter_url= 不带值的情况）
	nonEmpty := make([]string, 0, len(chapterURLs))
	for _, u := range chapterURLs {
		if u != "" {
			nonEmpty = append(nonEmpty, u)
		}
	}

	var result *model.CommentListResponse
	var err error
	switch {
	case len(nonEmpty) > 1:
		result, err = h.commentService.GetByChapterURLs(nonEmpty, c.Query("book_name"), page, pageSize)
	case len(nonEmpty) == 1:
		result, err = h.commentService.GetByChapter(nonEmpty[0], c.Query("book_name"), page, pageSize)
	case c.Query("book_name") != "":
		result, err = h.commentService.GetByBook(c.Query("book_name"), page, pageSize)
	default:
		result, err = h.commentService.GetAll(page, pageSize)
	}
	if err != nil {
		errcode.Respond(c, err, "获取评论列表失败")
		return
	}

	errcode.Success(c, result)
}

// GetMyComments 获取我的评论列表
// @Summary 获取我的评论列表
// @Description 获取当前用户的评论列表
// @Tags 评论
// @Produce json
// @Security ApiKeyAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /api/comments/my [get]
func (h *CommentHandler) GetMyComments(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	result, err := h.commentService.GetByUserID(userID, page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "获取评论列表失败")
		return
	}

	errcode.Success(c, result)
}

// Delete 删除评论
// @Summary 删除评论
// @Description 删除指定评论
// @Tags 评论
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "评论ID"
// @Success 200 {object} model.Response
// @Router /api/comments/{id} [delete]
func (h *CommentHandler) Delete(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		errcode.Error(c, errcode.BadRequest, "无效的评论ID")
		return
	}

	if err := h.commentService.Delete(uint(id), userID); err != nil {
		errcode.Respond(c, err, "删除评论失败")
		return
	}

	errcode.SuccessMsg(c, "删除成功", nil)
}

// MigrateKey 迁移评论聚合键
// @Summary 迁移评论聚合键
// @Description 将当前用户在旧聚合键下的评论批量迁移到新聚合键（合并书籍场景）
// @Tags 评论
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body model.MigrateCommentKeyRequest true "迁移请求"
// @Success 200 {object} model.Response
// @Router /api/comments/migrate-key [post]
func (h *CommentHandler) MigrateKey(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	var req model.MigrateCommentKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	count, err := h.commentService.MigrateKey(userID, req.OldKey, req.NewKey)
	if err != nil {
		errcode.Respond(c, err, "迁移评论聚合键失败")
		return
	}

	errcode.Success(c, model.MigrateCommentKeyResponse{MigratedCount: count})
}
