package middleware

import (
	"ebook-server/model"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery panic 恢复中间件
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 记录错误日志
				zap.L().Error("panic recovered",
					zap.Any("error", err),
					zap.String("stack", string(debug.Stack())),
				)

				// 返回错误响应
				model.Error(c, http.StatusInternalServerError, "服务器内部错误")
				c.Abort()
			}
		}()

		c.Next()
	}
}
