package handler

import (
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
)

// AccountHandler 账号生命周期 HTTP 处理器（数据导出与账号注销，ADR-0005）。
//
// 与 UserHandler（用户资料操作）分离：两组功能因不同原因变化——
// 资料操作随用户偏好变动，账号操作随账号安全策略变动。
type AccountHandler struct {
	accountService *service.AccountService
}

// NewAccountHandler 创建账号生命周期处理器实例。
func NewAccountHandler(accountService *service.AccountService) *AccountHandler {
	return &AccountHandler{
		accountService: accountService,
	}
}

// ExportMyData 导出当前用户的自有数据
// @Summary 导出我的数据
// @Description 返回用户资料与本人评论的完整副本，不含密码与操作日志（ADR-0005）
// @Tags 用户
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} model.Response
// @Router /api/users/me/data [get]
func (h *AccountHandler) ExportMyData(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	data, err := h.accountService.Export(userID)
	if err != nil {
		errcode.Respond(c, err, "导出用户数据失败")
		return
	}

	errcode.Success(c, data)
}

// SendDeletionCode 发送账号注销验证码到当前账号的邮箱
// @Summary 发送注销验证码
// @Description 向当前登录账号的邮箱发送 6 位注销验证码（ADR-0005）
// @Tags 用户
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} model.Response
// @Router /api/users/me/deletion/send-code [post]
func (h *AccountHandler) SendDeletionCode(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	if err := h.accountService.SendDeletionCode(userID); err != nil {
		errcode.Respond(c, err, "发送注销验证码失败")
		return
	}

	errcode.Success(c, nil)
}

// DeleteAccount 注销账号
// @Summary 注销账号
// @Description 校验邮箱验证码后匿名化账号，并在响应中返回用户数据副本。此操作不可撤销（ADR-0005）
// @Tags 用户
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body model.DeleteAccountRequest true "注销请求"
// @Success 200 {object} model.Response
// @Router /api/users/me/deletion [post]
func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	var req model.DeleteAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	data, err := h.accountService.Delete(userID, req.Code)
	if err != nil {
		errcode.Respond(c, err, "注销账号失败")
		return
	}

	errcode.Success(c, data)
}
