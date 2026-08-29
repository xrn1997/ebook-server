// Package errcode 定义五位字符串业务码与统一响应辅助函数。
//
// 所有业务端点 HTTP 状态码一律返回 200，成功与失败通过 code + error 表述。
package errcode

import (
	"net/http"

	"ebook-server/model"

	"github.com/gin-gonic/gin"
)

// 五位字符串业务码。成功恒为 "00000"。
// 所有业务端点 HTTP 状态码一律返回 200，错误通过 code + error 表述。
const (
	SuccessCode     = "00000" // 成功
	UsernameInvalid = "A0110" // 用户名格式不合规
	UsernameExists  = "A0111" // 用户名已存在
	EmailExists     = "A0112" // 邮箱已被注册
	PasswordInvalid = "A0120" // 密码格式不合规
	CodeInvalid     = "A0132" // 验证码错误
	EmailInvalid    = "A0153" // 邮箱格式错误
	AccountNotFound = "A0201" // 账户不存在
	PasswordWrong   = "A0210" // 密码错误
	LoginExpired    = "A0230" // 登录过期
	TokenInvalid    = "A0240" // token 无效
	AttemptTooMany  = "A0241" // 尝试超限
	AccountLocked   = "A0242" // 账号已锁定
	MailSendFailed  = "C0503" // 邮件发送失败

	// 通用兜底错误
	BadRequest  = "A0400" // 请求参数错误
	NotFound    = "A0404" // 资源不存在
	Forbidden   = "A0403" // 无权限
	ServerError = "C0500" // 服务器内部错误
)

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, model.Response{
		Code:  SuccessCode,
		Error: "",
		Data:  data,
	})
}

// Error 业务错误响应（HTTP 恒为 200）
func Error(c *gin.Context, code, message string) {
	c.JSON(http.StatusOK, model.Response{
		Code:  code,
		Error: message,
	})
}

// SuccessMsg 成功响应（带提示文案）
func SuccessMsg(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code":  SuccessCode,
		"error": message,
		"data":  data,
	})
}
