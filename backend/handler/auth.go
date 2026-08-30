// Package handler 实现 HTTP API 处理器，负责请求解析、参数校验与响应封装。
package handler

import (
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
)

// AuthHandler 认证相关 HTTP 处理器。
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler 创建认证处理器实例。
func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// SendCode 按邮箱发送注册验证码
// @Summary 发送验证码
// @Description 按邮箱发送 6 位注册验证码
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.SendCodeRequest true "发送验证码请求"
// @Success 200 {object} model.Response
// @Router /api/auth/send-code [post]
func (h *AuthHandler) SendCode(c *gin.Context) {
	var req model.SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := h.authService.SendCode(req.Email); err != nil {
		errcode.Respond(c, err, "发送验证码失败")
		return
	}

	errcode.Success(c, nil)
}

// Register 注册（邮箱+验证码+密码），激活建号，不发 token
// @Summary 注册
// @Description 校验邮箱验证码并创建已激活账号，不发 token
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.RegisterRequest true "注册请求"
// @Success 200 {object} model.Response
// @Router /api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	if _, err := h.authService.Register(&req); err != nil {
		errcode.Respond(c, err, "注册失败")
		return
	}

	// 注册成功：不发 token，客户端应引导用户去登录
	errcode.Success(c, nil)
}

// loginFailedMessage 登录失败对外的统一文案（ADR-0006）。
// 账号不存在与密码错误必须返回完全一致的响应——业务码与文案都不能有差异。
// 只统一业务码而保留差异文案，攻击者仍可凭 error 字段枚举已注册邮箱。
const loginFailedMessage = "邮箱或密码错误"

// Login 登录（邮箱+密码），签发双 token
// @Summary 登录
// @Description 邮箱+密码登录，返回双 token
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.LoginRequest true "登录请求"
// @Success 200 {object} model.Response
// @Router /api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	pair, err := h.authService.Login(&req)
	if err != nil {
		switch err {
		case model.ErrAccountNotFound, model.ErrPasswordWrong:
			// 覆盖默认映射：ErrAccountNotFound 在 errorCodes 表里落 A0201，
			// 但登录端点未认证，回 A0201 会让攻击者据此枚举已注册邮箱（ADR-0006）；
			// 且文案必须统一，不能透传 err.Error()（"账户不存在" / "密码错误"）。
			errcode.Error(c, errcode.PasswordWrong, loginFailedMessage)
		default:
			errcode.Respond(c, err, "登录失败")
		}
		return
	}

	errcode.Success(c, pair)
}

// Refresh 刷新 token
// @Summary 刷新 token
// @Description 使用 refresh token 换发新双 token（轮换）
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.RefreshRequest true "刷新请求"
// @Success 200 {object} model.Response
// @Router /api/auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	pair, err := h.authService.Refresh(req.RefreshToken)
	if err != nil {
		errcode.Respond(c, err, "刷新失败")
		return
	}

	errcode.Success(c, pair)
}

// Logout 登出
// @Summary 登出
// @Description 作废当前用户的所有 refresh token
// @Tags 认证
// @Security ApiKeyAuth
// @Success 200 {object} model.Response
// @Router /api/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	uid, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	if err := h.authService.Logout(uid); err != nil {
		errcode.Respond(c, err, "登出失败")
		return
	}

	errcode.Success(c, nil)
}

// ForgotPasswordSendCode 忘记密码发送验证码
// @Summary 忘记密码发送验证码
// @Description 校验账号存在后向邮箱发送验证码（不存在也返回成功，防枚举）
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.SendCodeRequest true "发送验证码请求"
// @Success 200 {object} model.Response
// @Router /api/auth/forgot-password/send-code [post]
func (h *AuthHandler) ForgotPasswordSendCode(c *gin.Context) {
	var req model.SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := h.authService.SendForgotCode(req.Email); err != nil {
		errcode.Respond(c, err, "发送验证码失败")
		return
	}

	errcode.Success(c, nil)
}

// ForgotPasswordReset 验证码重置密码
// @Summary 验证码重置密码
// @Description 使用邮箱验证码重置密码
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.ResetPasswordRequest true "重置密码请求"
// @Success 200 {object} model.Response
// @Router /api/auth/forgot-password/reset [post]
func (h *AuthHandler) ForgotPasswordReset(c *gin.Context) {
	var req model.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := h.authService.ResetPassword(req.Email, req.Code, req.NewPassword); err != nil {
		if err == model.ErrAccountNotFound {
			// 覆盖默认映射：本端点未认证，不返回 A0201（ADR-0006——该码仅用于
			// 已认证上下文）。且发码时已对未注册邮箱静默跳过，
			// "验证码通过但账号不存在"属异常态，对外一律按服务器错误处理。
			errcode.Error(c, errcode.ServerError, "重置密码失败")
			return
		}
		errcode.Respond(c, err, "重置密码失败")
		return
	}

	errcode.Success(c, nil)
}
