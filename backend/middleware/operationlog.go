package middleware

import (
	"strings"

	"ebook-server/model"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// OperationLogWriter operation_logs 写入能力（consumer-defined，ADR-0007）。
// 由 repository.LogRepository 满足。
type OperationLogWriter interface {
	Create(log *model.OperationLog) error
}

// OperationLog 操作审计中间件：把每次请求写入 operation_logs，供后台审计查看。
//
// 刻意**不写 RequestBody**——登录等请求体含明文密码（见 AGENTS.md 已知问题）。
// 写入失败只告警不阻断请求，避免日志链路影响业务。
// 用户身份在 c.Next() 期间由 JWTAuth 注入（保护路由内），故在此之后读取。
//
// 过滤：以 /admin 开头的是后台管理系统自身的请求（后台 UI/API 的访问），
// 审计聚焦**客户端**（android-ebook 等）的请求，故不记录。
func OperationLog(writer OperationLogWriter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// 只审计客户端请求；后台管理系统自身的访问不入库
		if strings.HasPrefix(c.Request.URL.Path, "/admin") {
			return
		}

		log := &model.OperationLog{
			Method:       c.Request.Method,
			Path:         c.Request.URL.Path,
			IP:           c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
			ResponseCode: c.Writer.Status(),
		}
		if uid, ok := GetCurrentUserID(c); ok {
			log.UserID = uid
		}
		if name, ok := GetCurrentUsername(c); ok {
			log.Username = name
		}

		if err := writer.Create(log); err != nil {
			zap.L().Warn("write operation log failed", zap.Error(err))
		}
	}
}
