package middleware

import (
	"net/http"

	"ebook-server/pkg/errcode"

	"github.com/gin-gonic/gin"
)

// BodyLimit 在进入 handler 前限制请求体大小。
//
// defaultMax 适用于挂载点下的所有请求；overrides 按**全路径精确匹配**
// （c.Request.URL.Path）为个别端点单开一档——头像上传与 JSON 是两个量级。
// 额度与覆盖表的路径都由装配方（路由表）传入，中间件里不抄第二份路径字面量。
// 覆盖表匹配不到实际登记的路径时，限额会静默落回默认档，因此路由表测试把
// 两侧钉成同一值（middleware 单测只证明本函数自己的行为）。
//
// JSON 解析器会按报文长度分配内存，不设上限时一个 Content-Length 很大的
// 请求就能把内存打满；这里在 Content-Length 已超限时直接拒绝，不必先把
// 报文读进来，chunked 由 MaxBytesReader 兜底。
//
// 与 Nginx 的 client_max_body_size 是两道独立的闸：边缘挡住公网，应用层兜住
// 「Nginx 配置漏了」或「有人绕过 Nginx 直连应用端口」这两种情况。
func BodyLimit(defaultMax int64, overrides map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := defaultMax
		if m, ok := overrides[c.Request.URL.Path]; ok {
			limit = m
		}

		if c.Request.ContentLength > limit {
			errcode.Error(c, errcode.BadRequest, "请求体过大")
			c.Abort()
			return
		}

		// ContentLength 未知（chunked）时也要封顶：读取超过 limit 后
		// MaxBytesReader 会让解析器报错，handler 按绑定失败返回 A0400。
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}

		c.Next()
	}
}
