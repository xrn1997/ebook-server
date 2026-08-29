package handler

import (
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService *service.UserService
	authService *service.AuthService
}

func NewUserHandler() *UserHandler {
	return &UserHandler{
		userService: service.NewUserService(),
		authService: service.NewAuthService(),
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
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	user, err := h.userService.GetByUID(userID)
	if err != nil {
		if err == model.ErrUserNotFound {
			errcode.Error(c, errcode.AccountNotFound, err.Error())
			return
		}
		errcode.Error(c, errcode.ServerError, "获取用户信息失败")
		return
	}

	errcode.Success(c, user)
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
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	user, err := h.userService.Update(userID, &req)
	if err != nil {
		if err == model.ErrUserNotFound {
			errcode.Error(c, errcode.AccountNotFound, err.Error())
			return
		}
		errcode.Error(c, errcode.ServerError, "更新用户信息失败")
		return
	}

	errcode.Success(c, user)
}

// ChangePassword 已登录修改密码
// @Summary 修改密码
// @Description 校验旧密码后更新密码，并使全部 token 失效
// @Tags 用户
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body model.ModifyPwdRequest true "改密请求"
// @Success 200 {object} model.Response
// @Router /api/users/me/password [put]
func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	var req model.ModifyPwdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := h.authService.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		if err == model.ErrPasswordWrong {
			errcode.Error(c, errcode.PasswordWrong, err.Error())
			return
		}
		errcode.Error(c, errcode.ServerError, "修改密码失败")
		return
	}

	errcode.Success(c, nil)
}
