package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"

	"github.com/gin-gonic/gin"
)

// RequestIDHeader 是请求/响应共用的关联 id 头。
const RequestIDHeader = "X-Request-ID"

// requestIDKey 是 gin context 中的存储键。
const requestIDKey = "request_id"

// 只接受客户端传来的安全字符；避免把换行/控制字符带进日志与响应头。
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// RequestID 为每个请求确定关联 id：合法客户端值优先，否则生成随机值。
//
// 生成的 id 同时写入响应头与 gin context，Logger 通过 GetRequestID 读取。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// GetRequestID 返回当前请求的关联 id；未经过 RequestID 中间件时返回空串。
func GetRequestID(c *gin.Context) string {
	value, ok := c.Get(requestIDKey)
	if !ok {
		return ""
	}
	id, _ := value.(string)
	return id
}

func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(buf[:])
}
