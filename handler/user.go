package handler

import (
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler() *UserHandler {
	return &UserHandler{
		userService: service.NewUserService(),
	}
}

// GetMe 获取当前用户信息
// @Summary 获取当前用户信息
// @Description 获取当前登录用户的信息
// @Tags 用户
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} model.Response
// @Router /api/users/me [get]
func (h *UserHandler) GetMe(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		model.Unauthorized(c, "未登录")
		return
	}

	user, err := h.userService.GetByID(userID)
	if err != nil {
		if err == model.ErrUserNotFound {
			model.NotFound(c, err.Error())
			return
		}
		model.InternalError(c, "获取用户信息失败")
		return
	}

	model.Success(c, user)
}

// UpdateMe 更新当前用户信息
// @Summary 更新当前用户信息
// @Description 更新当前登录用户的信息
// @Tags 用户
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body model.UpdateUserRequest true "更新请求"
// @Success 200 {object} model.Response
// @Router /api/users/me [put]
func (h *UserHandler) UpdateMe(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		model.Unauthorized(c, "未登录")
		return
	}

	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	user, err := h.userService.Update(userID, &req)
	if err != nil {
		if err == model.ErrUserNotFound {
			model.NotFound(c, err.Error())
			return
		}
		model.InternalError(c, "更新用户信息失败")
		return
	}

	model.Success(c, user)
}
