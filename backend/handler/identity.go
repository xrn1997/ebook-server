package handler

import (
	"ebook-server/middleware"
	"ebook-server/pkg/errcode"

	"github.com/gin-gonic/gin"
)

// CurrentUser 取当前请求的登录身份（账号根标识）。
//
// 未登录时由本函数写回 A0230 并返回 false，调用方只需 `if !ok { return }`。
// 「未登录」的出口只在这里一处：此前每个需要登录的处理器各抄一份同样的三行（改造前
// 共十二处），改文案或改业务码要把它们找齐，漏掉的那处会静默回出 C0500。
func CurrentUser(c *gin.Context) (uid uint, ok bool) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return 0, false
	}
	return userID, true
}
