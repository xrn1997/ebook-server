// Package errcode 定义五位字符串业务码与统一响应辅助函数。
//
// 所有业务端点 HTTP 状态码一律返回 200，成功与失败通过 code + error 表述。
package errcode

import (
	"errors"
	"net/http"

	"ebook-server/model"
	"ebook-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 五位字符串业务码。成功恒为 "00000"。
// 所有业务端点 HTTP 状态码一律返回 200，错误通过 code + error 表述。
const (
	SuccessCode     = model.SuccessCode // 成功；与数据层共用同一常量（见 model.SuccessCode）
	UsernameInvalid = "A0110"           // 用户名格式不合规
	UsernameExists  = "A0111"           // 用户名已存在
	EmailExists     = "A0112"           // 邮箱已被注册
	EmailImmutable  = "A0113"           // 登录主标识不可变更（ADR-0004）
	PasswordInvalid = "A0120"           // 密码格式不合规
	CodeInvalid     = "A0132"           // 验证码错误
	EmailInvalid    = "A0153"           // 邮箱格式错误
	AccountNotFound = "A0201"           // 账户不存在
	PasswordWrong   = "A0210"           // 密码错误
	LoginExpired    = "A0230"           // 登录过期
	TokenInvalid    = "A0240"           // token 无效
	AttemptTooMany  = "A0241"           // 尝试超限
	AccountLocked   = "A0242"           // 账号已锁定
	MailSendFailed  = "C0503"           // 邮件发送失败

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
// 表里只登记**有产生方**的 sentinel——没有产生方的哨兵会把「谁在产生这个错误」
// 变成读代码时猜不出来的事（ADR-0001 已把用户名重复、通用无权限等哨兵随之删除；
// 对应的业务码常量保留，因为它们是对外词表的一部分）。
//
// 注意：个别端点需要**覆盖**默认映射（例如登录为防枚举把「账号不存在」
// 映射成 A0210，见 ADR-0006）。覆盖用 Map 写在调用点并注明理由——
// 只改这张表而不同步端点，会静默破坏防枚举。
var errorCodes = []struct {
	err  error
	code string
}{
	{model.ErrEmailExists, EmailExists},
	{model.ErrEmailImmutable, EmailImmutable},
	{model.ErrCodeInvalid, CodeInvalid},
	{model.ErrAccountNotFound, AccountNotFound},
	{model.ErrPasswordWrong, PasswordWrong},
	{model.ErrLoginExpired, LoginExpired},
	{model.ErrAttemptTooMany, AttemptTooMany},
	{model.ErrAccountLocked, AccountLocked},
	{model.ErrMailSendFailed, MailSendFailed},
	{model.ErrCommentNotFound, CommentNotFoundErr},
	{model.ErrCommentNotOwner, CommentNotOwner},
	{model.ErrCommentKeySame, CommentKeySame},
	// ErrCommentKeyInvalid 是改键模块对「键本身不合规」的判定，与 binding 拒绝同码：
	// 客户端不必区分是谁挡下的（A0400）。
	{model.ErrCommentKeyInvalid, BadRequest},
	// 读路径的键上限同码：客户端要区分的是「参数不合规」，不是谁判下的。
	{model.ErrCommentFilterInvalid, BadRequest},
	// 密码超出 bcrypt 的字节上限：binding 的 max 按字符计，多字节密码可能通过校验
	// 却在哈希时失败；这条约束由凭据模块判定，业务码属「密码格式不合规」（A0120）。
	{model.ErrPasswordInvalid, PasswordInvalid},

	// 头像不合规由 pkg/upload 在生产、以 model.ErrInvalidImage 表达：登记在这里而不是
	// 各 handler 手映射——此前这条错误从未进过表，端点靠 `err == ...` 自己判，
	// 漏判即静默落 C0500（本表存在的全部理由）。哨兵放 model 而不是能力包，
	// 是为了不让错误映射反向依赖 pkg/upload。
	{model.ErrInvalidImage, BadRequest},
}

// From 返回 err 对应的业务码，未登记的 err 一律落通用码 ServerError。
//
// errors.Is 会走完整条包装链，因此 service 用 %w 加上下文的错误仍能拿到正确
// 业务码——此前是 `errorCodes[err]` 的同一性查表，等于在每个 service 的
// interface 上偷偷写一条「领域错误一律不许包装」的约束，破了的静默落 C0500。
//
// 一条链上同时出现两枚已登记 sentinel 时按表序取先者；今天 sentinel 都是叶子，
// 不存在这种链，不为它加逐层剥离的间接层。
//
// 未登记的错误不能回传其 error 文案（可能含驱动/网络细节），
// 调用方应对 ServerError 使用兜底文案（见 Respond）。
func From(err error) string {
	for _, entry := range errorCodes {
		if errors.Is(err, entry.err) {
			return entry.code
		}
	}
	return ServerError
}

// Override 端点级的映射覆盖：某条 sentinel 在这个端点要回另一个业务码与文案。
type Override struct {
	err     error
	code    string
	message string
}

// Map 声明一条覆盖。用于防枚举这类「同一枚错误在不同端点必须不同响」的场景
// （ADR-0006：未认证端点不能回 A0201，也不能透出差异文案）。
func Map(err error, code, message string) Override {
	return Override{err: err, code: code, message: message}
}

// Respond 把 err 翻译成统一信封：已登记的 err 透出其文案，未登记的只回兜底文案。
//
// 这是 handler 处理业务错误的标准写法：
//
//	errcode.Respond(c, err, "获取用户信息失败")
//
// 需要覆盖默认映射的端点把覆盖声明在调用点，而不是另抄一份 switch：
//
//	errcode.Respond(c, err, loginFailedMessage,
//		errcode.Map(model.ErrAccountNotFound, errcode.PasswordWrong, loginFailedMessage))
//
// 覆盖按声明顺序匹配（含包装链），命中即用其业务码与文案。
func Respond(c *gin.Context, err error, fallbackMessage string, overrides ...Override) {
	for _, o := range overrides {
		if errors.Is(err, o.err) {
			Error(c, o.code, o.message)
			return
		}
	}

	code := From(err)
	message := fallbackMessage
	if code != ServerError {
		message = err.Error()
	} else {
		// 未登记的错误不能回原始文案，但也不能让细节凭空消失：驱动/网络故障
		// 只留在服务端日志里，客户端拿到的是兜底文案。
		logger.Error("未登记的业务错误", zap.Error(err))
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
