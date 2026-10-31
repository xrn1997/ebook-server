// Package middleware 提供 Gin 中间件：JWT 认证、CORS、请求日志与 panic 恢复。
package middleware

import (
	"ebook-server/pkg/errcode"
	"ebook-server/pkg/jwt"
	"strings"

	"github.com/gin-gonic/gin"
)

// Parser 解析用户 access token 的能力（consumer-defined：本中间件只需要解析，
// 不需要签发）。声明成接口而不是收 *jwt.TokenBox，是为了让路由表的用例注入
// 「永远拒绝」与「永远放行」两种 adapter，从而分别断言鉴权挂在路上。
type Parser interface {
	ParseUser(tokenString string) (*jwt.Claims, error)
}

// JWTAuth 用户 access token 认证中间件。
//
// token 解析由装配点注入的 Parser 完成（不再有「配置已加载」这条隐式前提）。
func JWTAuth(tokens Parser) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Header 获取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			errcode.Error(c, errcode.LoginExpired, "缺少认证令牌")
			c.Abort()
			return
		}

		// 解析 Bearer Token
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			errcode.Error(c, errcode.LoginExpired, "认证格式错误")
			c.Abort()
			return
		}

		// 解析 Token
		claims, err := tokens.ParseUser(parts[1])
		if err != nil {
			errcode.Error(c, errcode.LoginExpired, "无效或过期的认证令牌")
			c.Abort()
			return
		}
		if claims.UserID == 0 {
			// 不指名账号的 token 不算已认证：uid 是主键，0 不可能是任何账号。
			// 数据层不再拿 0 当「不限用户」的哨兵（评论读路径改用显式 ByUserID），
			// 但下游仍处处假定「当前账号」非 0；这一条让那个假定在入口处成立。
			errcode.Error(c, errcode.LoginExpired, "无效或过期的认证令牌")
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)

		c.Next()
	}
}

// GetCurrentUserID 获取当前用户 ID
func GetCurrentUserID(c *gin.Context) (uint, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	id, ok := userID.(uint)
	return id, ok
}

// GetCurrentUsername 获取当前用户名
func GetCurrentUsername(c *gin.Context) (string, bool) {
	username, exists := c.Get("username")
	if !exists {
		return "", false
	}
	name, ok := username.(string)
	return name, ok
}
