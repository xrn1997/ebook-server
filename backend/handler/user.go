package handler

import (
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
	profileService *service.ProfileService
	authService    *service.AuthService
}

// NewUserHandler 创建用户处理器实例。
func NewUserHandler(profileService *service.ProfileService, authService *service.AuthService) *UserHandler {
	return &UserHandler{
		profileService: profileService,
		authService:    authService,
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
	userID, signedIn := CurrentUser(c)
	if !signedIn {
		return
	}

	user, err := h.profileService.Get(c.Request.Context(), userID)
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
	userID, signedIn := CurrentUser(c)
	if !signedIn {
		return
	}

	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.RespondBindError(c, err, "更新资料")
		return
	}

	user, err := h.profileService.Update(c.Request.Context(), userID, &req)
	if err != nil {
		errcode.Respond(c, err, "更新用户信息失败")
		return
	}

	errcode.Success(c, user)
}

// ChangePassword 已登录修改密码
// @Summary 修改密码
// @Description 校验旧密码后更新密码，并撤销该用户的全部 refresh token（access token 无状态，活到自身过期）
// @Tags 用户
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body model.ModifyPwdRequest true "改密请求"
// @Success 200 {object} model.Response
// @Router /api/users/me/password [put]
func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID, signedIn := CurrentUser(c)
	if !signedIn {
		return
	}

	var req model.ModifyPwdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.RespondBindError(c, err, "修改密码")
		return
	}

	if err := h.authService.ChangePassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		errcode.Respond(c, err, "修改密码失败")
		return
	}

	errcode.Success(c, nil)
}
