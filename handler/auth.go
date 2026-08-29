package handler

import (
	"ebook-server/model"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		authService: service.NewAuthService(),
	}
}

// Register 用户注册
// @Summary 用户注册
// @Description 创建新用户账号
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.RegisterRequest true "注册请求"
// @Success 200 {object} model.Response
// @Router /api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	user, err := h.authService.Register(&req)
	if err != nil {
		if err == model.ErrUsernameExists {
			model.BadRequest(c, err.Error())
			return
		}
		model.InternalError(c, "注册失败")
		return
	}

	model.Success(c, gin.H{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
	})
}

// Login 用户登录
// @Summary 用户登录
// @Description 用户登录获取令牌
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body model.LoginRequest true "登录请求"
// @Success 200 {object} model.Response
// @Router /api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		model.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}

	token, user, err := h.authService.Login(&req)
	if err != nil {
		if err == model.ErrInvalidCredentials {
			model.Unauthorized(c, err.Error())
			return
		}
		model.InternalError(c, "登录失败")
		return
	}

	model.Success(c, gin.H{
		"token":    token,
		"username": user.Username,
		"email":    user.Email,
	})
}
