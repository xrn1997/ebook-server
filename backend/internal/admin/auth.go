package admin

import (
	"crypto/sha256"
	"crypto/subtle"
	"time"

	"ebook-server/pkg/errcode"
	"ebook-server/pkg/httpx"
	pubjwt "ebook-server/pkg/jwt"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// sum 定长摘要：让常量时间比较不再按输入长度分档。
func sum(in string) []byte {
	h := sha256.Sum256([]byte(in))
	return h[:]
}

// roleAdmin 后台 token 必须携带的角色声明。
const roleAdmin = "admin"

// adminClaims 管理端 JWT 声明。与公开用户 JWT（pkg/jwt.Claims）完全分离：不同的
// 声明集、不同的签发密钥（admin.jwt_secret），公开 access token 与后台 token 互不
// 通用（ADR-0001 §认证模型、ADR-0010 网络隔离）。
type adminClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Session 后台的凭据校验与 token 生命周期。与用户侧共用 pkg/jwt 的 TokenBox：
// 两把密钥、两种声明，一套签发/验签规则——「密钥是谁、有效期多长、接受哪种算法」
// 在构造时定死，安全前提不随两份实现各自漂移。
type Session struct {
	username string
	password string
	tokens   *pubjwt.TokenBox
}

// SessionConfig 是 NewSession 的构造输入：会话只需要凭据与有效期四项。刻意不复用
// config.AdminConfig：监听地址之类属于装配层（main.go）的知识，admin 包不该知道
// 配置里还有什么，配置 → 这四项的转换留在那个装配点。
type SessionConfig struct {
	Username  string
	Password  string
	JWTSecret string
	ExpireMin int
}

// NewSession 按会话配置装配。有效期原样取自调用方（默认值由 config 层给出）。
func NewSession(cfg SessionConfig) *Session {
	return &Session{
		username: cfg.Username,
		password: cfg.Password,
		tokens:   pubjwt.New(cfg.JWTSecret, cfg.ExpireMin),
	}
}

// Login 校验管理端账号密码，签发管理端 token。两次都比较**定长摘要**：直接比明文
// 会让耗时随输入长度走，把口令长度变成可测的毫秒档；SHA-256 后两边都是 32 字节。
// 这里不需要 bcrypt——口令来自配置文件、每次登录只比对一次，抗穷举靠的是后台监听
// 隔离与登录限流（ADR-0010），不靠这次比较。
func (s *Session) Login(username, password string) (string, error) {
	userOK := subtle.ConstantTimeCompare(sum(username), sum(s.username)) == 1
	pwdOK := subtle.ConstantTimeCompare(sum(password), sum(s.password)) == 1
	if !userOK || !pwdOK {
		return "", ErrCredInvalid
	}

	now := time.Now()
	return s.tokens.Issue(&adminClaims{
		Role: roleAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokens.Expire())),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    pubjwt.Issuer,
		},
	})
}

// validate 校验管理端 token：签名与密钥正确且 role 为 admin。
func (s *Session) validate(tokenString string) error {
	claims := &adminClaims{}
	if err := s.tokens.Parse(tokenString, claims); err != nil {
		return ErrTokenInvalid
	}
	if claims.Role != roleAdmin {
		return ErrTokenInvalid
	}
	return nil
}

// AuthMiddleware 管理端 API 认证中间件。Bearer 头的形状判定与用户侧共用
// httpx.BearerToken，但两把密钥永不互认（ADR-0010）；形状错了与没带 token 在这里
// 回同一句话（A0403），失败码也不同于公开侧的 A0230。
func (s *Session) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := httpx.BearerToken(c.GetHeader("Authorization"))
		if err != nil {
			errcode.Error(c, adminAuthFailureCode, "缺少管理端认证令牌")
			c.Abort()
			return
		}
		if err := s.validate(token); err != nil {
			errcode.Error(c, adminAuthFailureCode, ErrTokenInvalid.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}
