package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Logger 请求日志中间件。
//
// 不读取 request body：这里从来只记录协议事实，把 body 全量读进内存再放回去
// 既有大小无上限的内存风险，也没有任何消费者。审计模块同样刻意不记 body
// （ADR-0018）；真需要请求内容时，应在具体端点里做一次显式、有上限的观察，
// 而不是让中间件对所有流量都留一份副本。
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 用 defer 而不是 c.Next() 之后的语句：handler panic 时中间件栈会直接
		// 向上展开，写在后面的日志就整条消失。defer 保证至少留下这一条访问记录。
		defer func() {
			zap.L().Info("HTTP Request",
				zap.String("request_id", GetRequestID(c)),
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Int("status", c.Writer.Status()),
				zap.Duration("latency", time.Since(start)),
				zap.String("client_ip", c.ClientIP()),
				zap.Int("body_size", c.Writer.Size()),
			)
		}()

		// 处理请求
		c.Next()
	}
}
