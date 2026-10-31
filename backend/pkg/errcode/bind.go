package errcode

import (
	"errors"
	"reflect"

	"ebook-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// bindFieldLabels 把校验失败的 Go 字段名翻成客户端界面上出现的中文名。
//
// 表里的键是 model 中请求结构体的字段名（validator 报的是它，不是 json tag）。
// 新增请求字段时不必登记：漏了就落到「该字段」这个兜底说法，功能不会坏，只是话含糊一点。
var bindFieldLabels = map[string]string{
	"Email":        "邮箱",
	"Code":         "验证码",
	"Password":     "密码",
	"OldPassword":  "原密码",
	"NewPassword":  "新密码",
	"Content":      "评论内容",
	"CommentKey":   "评论聚合键",
	"ChapterURL":   "章节地址",
	"ChapterName":  "章节名",
	"BookName":     "书名",
	"Nickname":     "昵称",
	"Avatar":       "头像地址",
	"OldKey":       "旧聚合键",
	"NewKey":       "新聚合键",
	"RefreshToken": "刷新令牌",
}

// bindActionFieldLabels 给同名字段在不同请求里补上下文。
//
// Username 在「后台登录」里是管理端登录名，在「更新资料」里是用户的展示用名；
// 两处共用同一个 Go 字段名，笼统译成「用户名」既踩 CONTEXT 的 _Avoid_，
// 也说不出用户到底该改哪个字段。
var bindActionFieldLabels = map[string]map[string]string{
	"后台登录": {"Username": "后台登录名"},
	"更新资料": {"Username": "展示用名"},
}

// RespondBindError 把请求绑定/校验失败回成 A0400 + 一句人话，原始错误只进日志。
//
// 为什么处理器不能再写 `"请求参数错误: " + err.Error()`：那是 go-playground/validator 的
// 英文原文（Key: 'CreateCommentRequest.Content' Error:Field validation for 'Content'
// failed on the 'max' tag），而统一信封里的 error 字段会被客户端原样 Toast 给用户——
// 用户于是看见 Go 结构体名与标签名。按 AGENTS.md「错误处理」条：进文案的只能是说给
// 人听的话，驱动/校验器/第三方系统的原始错误一律就地写日志。
func RespondBindError(c *gin.Context, err error, action string) {
	logger.Warn("请求解析失败", zap.String("action", action), zap.Error(err))

	var ves validator.ValidationErrors
	if errors.As(err, &ves) && len(ves) > 0 {
		Error(c, BadRequest, bindMessage(ves[0], action))
		return
	}
	// 非 validator 的解析错误（JSON 语法错、字段类型不符等）同样不回原文：
	// 编码器的报错里带着它内部如何解析这份报文。
	Error(c, BadRequest, "请求格式不正确")
}

// bindMessage 把单条校验失败说成「哪个字段、怎么不对」。
func bindMessage(fe validator.FieldError, action string) string {
	label, ok := bindActionFieldLabels[action][fe.Field()]
	if !ok {
		label, ok = bindFieldLabels[fe.Field()]
	}
	if !ok {
		label = "该字段"
	}
	switch fe.Tag() {
	case "required":
		return "请填写" + label
	case "min":
		if isNumericField(fe) {
			return label + "不能小于 " + fe.Param()
		}
		return label + "长度不能少于 " + fe.Param() + " 字"
	case "max":
		if isNumericField(fe) {
			return label + "不能大于 " + fe.Param()
		}
		return label + "长度不能超过 " + fe.Param() + " 字"
	case "len":
		if isNumericField(fe) {
			return label + "必须等于 " + fe.Param()
		}
		return label + "长度必须为 " + fe.Param() + " 字"
	case "email":
		return label + "格式不正确"
	case "url":
		return label + "不是合法的链接"
	default:
		return label + "不符合要求"
	}
}

// isNumericField 判断被校验的是数值还是字符串。
//
// min/max/len 这三条标签对两者含义不同（长度上下限 vs 取值范围），
// validator 报的是同一个标签名，只能回到字段的 reflect.Kind 上区分。
func isNumericField(fe validator.FieldError) bool {
	if fe.Type() == nil {
		return false
	}
	switch fe.Type().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
