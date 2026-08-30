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
// @Description 获取评论列表，可按章节过滤（chapter_url/book_name）
// @Tags 评论
// @Produce json
// @Param chapter_url query string false "书源章节 URL（提供则返回该章节评论）"
// @Param book_name query string false "书名（与 chapter_url 配合二次过滤）"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /api/comments [get]
func (h *CommentHandler) GetList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	// 章节/书名过滤（ADR-0011）：chapter_url 精确匹配章节，book_name 可单独过滤全书
	var result *model.CommentListResponse
	var err error
	switch {
	case c.Query("chapter_url") != "":
		result, err = h.commentService.GetByChapter(c.Query("chapter_url"), c.Query("book_name"), page, pageSize)
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
