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
	EmailImmutable  = "A0113" // 登录主标识不可变更（ADR-0004）
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

	// 评论域业务码（ADR-0011）
	CommentContentEmpty   = "A0301" // 评论内容为空（预留：content 由 binding required 兜底，暂不启用）
	CommentChapterInvalid = "A0302" // chapter_url 非法/章节不存在（预留：后端无书源数据，不做存在性校验）
	CommentNotOwner       = "A0303" // 无权删除评论（非本人）
	CommentNotFoundErr    = "A0304" // 评论不存在
	CommentKeySame        = "A0305" // 新旧聚合键相同

	// 通用兜底错误
	BadRequest  = "A0400" // 请求参数错误
	NotFound    = "A0404" // 资源不存在
	Forbidden   = "A0403" // 无权限
	ServerError = "C0500" // 服务器内部错误
)

// errorCodes 领域 sentinel 错误到业务码的唯一映射表。
//
// 此前这份映射散落在各 handler 的手写 switch 里，新增错误要改多处，
// 漏一处就静默退化成 C0500。集中成表后，handler 一律走 From/Respond。
//
// 注意：个别端点需要**覆盖**默认映射（例如登录为防枚举把「账号不存在」
// 映射成 A0210，见 ADR-0006）。覆盖必须写在调用点并注明理由——
// 只改这张表而不同步端点，会静默破坏防枚举。
var errorCodes = map[error]string{
	model.ErrUsernameExists:  UsernameExists,
	model.ErrEmailExists:     EmailExists,
	model.ErrEmailImmutable:  EmailImmutable,
	model.ErrCodeInvalid:     CodeInvalid,
	model.ErrAccountNotFound: AccountNotFound,
	model.ErrUserNotFound:    AccountNotFound,
	model.ErrPasswordWrong:   PasswordWrong,
	model.ErrLoginExpired:    LoginExpired,
	model.ErrAttemptTooMany:  AttemptTooMany,
	model.ErrAccountLocked:   AccountLocked,
	model.ErrMailSendFailed:  MailSendFailed,
	model.ErrCommentNotFound: CommentNotFoundErr,
	model.ErrCommentNotOwner: CommentNotOwner,
	model.ErrCommentKeySame:  CommentKeySame,
	model.ErrNoPermission:    Forbidden,
}

// From 返回 err 对应的业务码，未登记的 err 一律落通用码 ServerError。
//
// 未登记的错误不能回传其 error 文案（可能含驱动/网络细节），
// 调用方应对 ServerError 使用兜底文案（见 Respond）。
func From(err error) string {
	if code, ok := errorCodes[err]; ok {
		return code
	}
	return ServerError
}

// Respond 把 err 翻译成统一信封：已登记的 err 透出其文案，未登记的只回兜底文案。
//
// 这是 handler 处理业务错误的标准写法：
//
//	errcode.Respond(c, err, "获取用户信息失败")
func Respond(c *gin.Context, err error, fallbackMessage string) {
	code := From(err)
	message := fallbackMessage
	if code != ServerError {
		message = err.Error()
	}
	Error(c, code, message)
}

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
