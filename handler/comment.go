package handler

import (
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

type CommentHandler struct {
	commentService *service.CommentService
}

func NewCommentHandler() *CommentHandler {
	return &CommentHandler{
		commentService: service.NewCommentService(),
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
		errcode.Error(c, errcode.ServerError, "创建评论失败")
		return
	}

	errcode.Success(c, comment)
}

// GetList 获取评论列表
// @Summary 获取评论列表
// @Description 获取所有评论列表
// @Tags 评论
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /api/comments [get]
func (h *CommentHandler) GetList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	result, err := h.commentService.GetAll(page, pageSize)
	if err != nil {
		errcode.Error(c, errcode.ServerError, "获取评论列表失败")
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
		errcode.Error(c, errcode.ServerError, "获取评论列表失败")
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
		if err == model.ErrCommentNotFound {
			errcode.Error(c, errcode.NotFound, err.Error())
			return
		}
		if err == model.ErrNoPermission {
			errcode.Error(c, errcode.Forbidden, err.Error())
			return
		}
		errcode.Error(c, errcode.ServerError, "删除评论失败")
		return
	}

	errcode.SuccessMsg(c, "删除成功", nil)
}
