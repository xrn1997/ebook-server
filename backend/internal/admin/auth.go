package admin

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"
	"time"

	"ebook-server/config"
	"ebook-server/pkg/errcode"
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

// adminClaims 管理端 JWT 声明。
//
// 与公开用户 JWT（pkg/jwt.Claims）完全分离：不同的声明集、不同的签发密钥
// （admin.jwt_secret），因此公开 access token 无法用于后台，后台 token 也无法用于
// 公开 API（ADR-0001 §认证模型、ADR-0010 网络隔离）。
type adminClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Session 后台的凭据校验与 token 生命周期。
//
// 它与用户侧共用 pkg/jwt 的 TokenBox 实现——两把密钥、两种声明，一套签发/验签规则。
// 此前后台另写了一份签发与解析（含算法白名单检查），两处的安全前提会各自漂移；
// 收成一个 Session 之后，「密钥是谁、有效期多长、接受哪种算法」在构造时就定死。
type Session struct {
	username string
	password string
	tokens   *pubjwt.TokenBox
}

// NewSession 按后台配置装配会话。有效期原样取自配置（默认值由 config 层给出）。
func NewSession(cfg config.AdminConfig) *Session {
	return &Session{
		username: cfg.Username,
		password: cfg.Password,
		tokens:   pubjwt.New(cfg.JWTSecret, cfg.ExpireMin),
	}
}

// Login 校验管理端账号密码，签发管理端 token。
//
// 两次都比较**定长摘要**：ConstantTimeCompare 没有短路，但耗时仍随输入长度走，
// 直接比明文会把「配置里的口令/账号有多长」变成可测的毫秒档。先 SHA-256 一次，
// 两边都是 32 字节，长度信息随之消失。这里不需要 bcrypt 那一层——口令来自配置文件、
// 每次登录只比对一次，抗穷举靠的是后台监听隔离与登录限流（ADR-0010），不是这次比较。
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

// AuthMiddleware 管理端 API 认证中间件。
//
// Bearer 头的解析形状与用户侧中间件一致，但两把密钥永不互认（ADR-0010）。
func (s *Session) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			errcode.Error(c, adminAuthFailureCode, "缺少管理端认证令牌")
			c.Abort()
			return
		}
		if err := s.validate(parts[1]); err != nil {
			errcode.Error(c, adminAuthFailureCode, ErrTokenInvalid.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}
