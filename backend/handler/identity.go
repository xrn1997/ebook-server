package handler

import (
	"ebook-server/middleware"
	"ebook-server/pkg/errcode"

	"github.com/gin-gonic/gin"
)

// CurrentUser 取当前请求的登录身份（账号根标识）。
//
// 未登录时由本函数写回 A0230 并返回 false，调用方只需 `if !ok { return }`。
// 「未登录」的出口只在这里一处：改文案或改业务码不会漏掉某个处理器，
// 不会有一处静默回出 C0500。
func CurrentUser(c *gin.Context) (uid uint, ok bool) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return 0, false
	}
	return userID, true
}
