package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"ebook-server/model"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// maxEnvelopeBytes 审计从响应体里最多读 4 KiB。
//
// 需要的是统一信封的头部（code/error 在最前面），而评论列表这类成功响应可以有
// 几十 KB。不设上限时，审计中间件会把每个响应的完整副本在内存里再存一份；
// 截断只会让超大成功响应的业务码缺失（空码不计入「只看失败」），不会影响分页结果。
const maxEnvelopeBytes = 4 << 10

// OperationRecorder 审计写入能力（consumer-defined，ADR-0007）。
// 由 service.LogService 满足：过滤规则（/admin*、/health）也在那里。
type OperationRecorder interface {
	Record(ctx context.Context, record model.OperationRecord) error
}

// recordWriter 有界拦截响应体，用于从统一信封里提取业务码/文案。
//
// 普通 HTTP 状态码恒为 200（见 ADR-0001），单看 response_code 无法区分业务成败；
// 故读取响应体拿到 {code,error}，把业务码与文案写进审计日志。
// 只捕获 JSON（非 JSON 响应没有信封），且最多 maxEnvelopeBytes 字节。
type recordWriter struct {
	gin.ResponseWriter
	body    bytes.Buffer
	capture bool
	checked bool
}

// Write 同时写入缓冲与真实响应；缓冲有上限、只认 JSON。
func (w *recordWriter) Write(b []byte) (int, error) {
	if !w.checked {
		w.checked = true
		w.capture = strings.Contains(w.Header().Get("Content-Type"), "application/json")
	}
	if w.capture && w.body.Len() < maxEnvelopeBytes {
		remaining := maxEnvelopeBytes - w.body.Len()
		chunk := b
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
		}
		_, _ = w.body.Write(chunk)
	}
	return w.ResponseWriter.Write(b)
}

// OperationLog 操作审计中间件：把每次请求交给审计模块。
//
// 刻意**不写 RequestBody**——登录等请求体含明文密码（见 ADR-0018）。
// 写入失败只告警不阻断请求，避免日志链路影响业务。
// 用户身份在 c.Next() 期间由 JWTAuth 注入（保护路由内），故在此之后读取。
//
// 中间件不判断「该不该记」：那是日志模块的规则（Record），挂载点只决定
// 「哪些表面接入了审计」。公开表面把它挂在 /api 组上，/health 与静态资源
// 因此不会经过这里。
func OperationLog(recorder OperationRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 拦截响应体以提取业务码
		bw := &recordWriter{ResponseWriter: c.Writer}
		c.Writer = bw

		// 与 Logger 同理：panic 时也要留下审计事实，不能在栈展开时整条丢掉。
		defer func() {
			record := model.OperationRecord{
				Method:       c.Request.Method,
				Path:         c.Request.URL.Path,
				IP:           c.ClientIP(),
				UserAgent:    c.Request.UserAgent(),
				ResponseCode: c.Writer.Status(),
			}
			if uid, ok := GetCurrentUserID(c); ok {
				record.UserID = uid
			}
			if name, ok := GetCurrentUsername(c); ok {
				record.Username = name
			}
			extractEnvelope(bw.body.Bytes(), &record)

			// 审计写入刻意脱离请求取消：客户端断开恰恰是最需要留下记录的请求，
			// 而 c.Request.Context() 此时已经取消，直接传给 DB 会让 INSERT 必然失败。
			// 用一个短超时兜住数据库卡死；这是 ADR-0022「请求作用域」边界之外的一次
			// 有意的静默操作，不能让它的失败跟着调用方走。
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
			defer cancel()
			if err := recorder.Record(ctx, record); err != nil {
				zap.L().Warn("write operation log failed", zap.Error(err))
			}
		}()

		c.Next()
	}
}

// extractEnvelope 从统一信封提取业务码与文案；非 JSON 或无 code 时置空。
func extractEnvelope(body []byte, record *model.OperationRecord) {
	var env struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if len(body) == 0 || json.Unmarshal(body, &env) != nil {
		return
	}
	record.ErrorCode = env.Code
	record.ErrorMessage = env.Error
}
