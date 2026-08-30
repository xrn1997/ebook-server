package admin

import (
	"crypto/subtle"
	"strings"
	"time"

	"ebook-server/config"
	"ebook-server/pkg/errcode"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// adminClaims 管理端 JWT 声明。
//
// 与公开用户 JWT（pkg/jwt.Claims）完全分离：不同的声明集、不同的签发密钥（admin.jwt_secret），
// 因此公开 access token 无法用于后台，后台 token 也无法用于公开 API。
type adminClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Login 校验管理端账号密码，签发管理端 token。
func Login(username, password string) (string, error) {
	cfg := config.AppConfig.Admin
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(cfg.Username)) == 1
	pwdOK := subtle.ConstantTimeCompare([]byte(password), []byte(cfg.Password)) == 1
	if !userOK || !pwdOK {
		return "", ErrCredInvalid
	}
	return issueToken()
}

// issueToken 以管理端实名密钥签发含 role=admin 的 token。
func issueToken() (string, error) {
	expire := config.AppConfig.Admin.ExpireMin
	claims := adminClaims{
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expire) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "ebook-server",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.AppConfig.Admin.JWTSecret))
}

// validate 校验管理端 token：签名与密钥正确且 role 为 admin。
func validate(tokenString string) error {
	token, err := jwt.ParseWithClaims(tokenString, &adminClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.AppConfig.Admin.JWTSecret), nil
	})
	if err != nil {
		return ErrTokenInvalid
	}
	claims, ok := token.Claims.(*adminClaims)
	if !ok || !token.Valid || claims.Role != "admin" {
		return ErrTokenInvalid
	}
	return nil
}

// AuthMiddleware 管理端 API 认证中间件。
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			errcode.Error(c, errcode.Forbidden, "缺少管理端认证令牌")
			c.Abort()
			return
		}
		if err := validate(parts[1]); err != nil {
			errcode.Error(c, errcode.Forbidden, ErrTokenInvalid.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}
