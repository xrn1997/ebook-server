package middleware

import (
	"bytes"
	"encoding/json"
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

// recordWriter 拦截响应体，用于从统一信封里提取业务码/文案。
//
// 普通 HTTP 状态码恒为 200（见 ADR-0001），单看 response_code 无法区分业务成败；
// 故读取响应体拿到 {code,error}，把业务码与文案写进审计日志。
type recordWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

// Write 同时写入缓冲与真实响应。
func (w *recordWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
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
		// 拦截响应体以提取业务码
		bw := &recordWriter{ResponseWriter: c.Writer}
		c.Writer = bw
		c.Next()

		// 只审计客户端请求；后台管理系统自身的访问不入库
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/admin") {
			return
		}

		log := &model.OperationLog{
			Method:       c.Request.Method,
			Path:         path,
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
		extractEnvelope(bw.body.Bytes(), log)

		if err := writer.Create(log); err != nil {
			zap.L().Warn("write operation log failed", zap.Error(err))
		}
	}
}

// extractEnvelope 从统一信封提取业务码与文案；非 JSON 或无 code 时置空。
func extractEnvelope(body []byte, log *model.OperationLog) {
	var env struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if len(body) == 0 || json.Unmarshal(body, &env) != nil {
		return
	}
	log.ErrorCode = env.Code
	log.ErrorMessage = env.Error
}
