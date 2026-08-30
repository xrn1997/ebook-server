package handler

import (
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户信息 HTTP 处理器。
//
// 负责用户资料操作（获取/更新信息、修改密码）；
// 账号生命周期（数据导出、注销）由 AccountHandler 负责。
// 头像文件清理由 service 层编排（ADR-0011），本层只做协议转换。
type UserHandler struct {
	userService *service.UserService
	authService *service.AuthService
}

// NewUserHandler 创建用户处理器实例。
func NewUserHandler(userService *service.UserService, authService *service.AuthService) *UserHandler {
	return &UserHandler{
		userService: userService,
		authService: authService,
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
		errcode.Respond(c, err, "获取用户信息失败")
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
		errcode.Respond(c, err, "更新用户信息失败")
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
		errcode.Respond(c, err, "修改密码失败")
		return
	}

	errcode.Success(c, nil)
}
