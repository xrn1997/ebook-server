// Package jwt 提供 access token 的签发与解析。
package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 用户侧 JWT 自定义声明，包含账号根标识与展示用名。
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Issuer 签发者标识，用户端与后台端共用同一枚值。
//
// 导出是因为后台要自填 RegisteredClaims（声明集不同），不导出它就得抄一份字面量——
// 而「两端一致」这种话一旦靠抄来维持，改的人会只改一边。
const Issuer = "ebook-server"

// ErrTokenRejected 验签过程整体通过但 token 被判为不可用。
//
// 本包自己的哨兵：调用方只需知道「拒了」，不需要依赖第三方库的错误命名。
var ErrTokenRejected = errors.New("认证令牌无效")

// signingMethod 本服务唯一接受的签名算法。
//
// 解析时显式固定算法：库默认会按 token 头部声明的算法验签，
// 攻击者把算法换成非对称公钥对应的算法时，用 HMAC 密钥去验签可能出现「无签名即通过」。
var signingMethod = jwt.SigningMethodHS256

// TokenBox 一把密钥加一个有效期构成的签发/解析单元。
//
// 密钥与 TTL 由构造函数传入而不是去读包级配置：如此「配置已加载」不再是每个
// 调用方 interface 上一条没人写出来的前提，测试也无需改写全局就能造出可用的 token。
// 用户端与后台端各持一个实例（两把互不可用的密钥，ADR-0001/0010），
// 算法、签发者、过期与验签规则只有一份实现。
type TokenBox struct {
	secret []byte
	expire time.Duration
}

// New 创建 token 单元。
//
// expireMin 原样生效（不在这里偷偷兜底成默认值）：默认值属配置层的职责，
// 见 config.setDefaults；把它抄进 TokenBox 会让「配了 0 分钟」与「没配」无法区分。
func New(secret string, expireMin int) *TokenBox {
	return &TokenBox{secret: []byte(secret), expire: time.Duration(expireMin) * time.Minute}
}

// Expire 本单元签出的 token 有效期。
//
// 给需要自己拼声明的调用方（后台 Session）用：TTL 已经由构造函数定在单元里，
// 调用方再存一份就成了「有效期到底是多久」的两个真相来源。
func (t *TokenBox) Expire() time.Duration { return t.expire }

// IssueUser 签发一枚用户 access token（短效，分钟级）。
func (t *TokenBox) IssueUser(userID uint, username string) (string, error) {
	now := time.Now()
	return t.Issue(&Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(t.expire)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    Issuer,
		},
	})
}

// ParseUser 解析用户 access token 并返回其声明。
func (t *TokenBox) ParseUser(tokenString string) (*Claims, error) {
	claims := &Claims{}
	if err := t.Parse(tokenString, claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// Issue 以本实例的密钥签发任意声明（后台端复用同一实现，声明集不同）。
func (t *TokenBox) Issue(claims jwt.Claims) (string, error) {
	return jwt.NewWithClaims(signingMethod, claims).SignedString(t.secret)
}

// Parse 验签并把声明解进 into；签名不符、算法不在白名单、缺过期或签发者不符都返回错误。
//
// 三条校验都得在这里，不能只靠「签的时候写了」：
//   - WithValidMethods 挡住算法混淆（见 signingMethod 的说明）。
//   - WithExpirationRequired：golang-jwt v5 只在声明**带了** exp 时才校验它，
//     一枚没有 exp 的 token 会因为「没有东西可过期」而永远通过。签发方总是写的，
//     但密钥一旦泄露或被人离线构造，缺 exp 就是免死金牌——要求它存在才是下限。
//   - WithIssuer：本包与后台共用这一份实现（pkg/jwt.Issuer 是两端同值的唯一来源）。
//     公开 access token 与后台 token 的真正隔离靠的是两把密钥与不同的声明集，
//     但类型注释已经把「签发者」算作这套规则的一部分，不校验就是注释与代码不一致。
func (t *TokenBox) Parse(tokenString string, into jwt.Claims) error {
	token, err := jwt.ParseWithClaims(tokenString, into,
		func(*jwt.Token) (interface{}, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(Issuer),
	)
	if err != nil {
		return err
	}
	if !token.Valid {
		// 库在 err==nil 时不会给出不 Valid 的 token；这条是防御性的双保险，
		// 不依赖库的错误类型（各版本命名不同）。
		return ErrTokenRejected
	}
	return nil
}
