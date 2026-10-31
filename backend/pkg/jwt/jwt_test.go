package jwt

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// newTestBox 造一把测试专用的 token 单元（密钥与有效期在构造时给定，无全局配置）。
func newTestBox(secret string, expireMin int) *TokenBox {
	return New(secret, expireMin)
}

func TestTokenBox_IssueUserShape(t *testing.T) {
	box := newTestBox("test-secret-key", 1)

	token, err := box.IssueUser(1, "testuser")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}
	if token == "" {
		t.Error("Token should not be empty")
	}
	if got := len(strings.Split(token, ".")); got != 3 {
		t.Errorf("Token should have 3 parts, got %d", got)
	}
}

func TestTokenBox_ParseUserRoundTrip(t *testing.T) {
	box := newTestBox("test-secret-key", 1)

	token, err := box.IssueUser(123, "admin")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}

	claims, err := box.ParseUser(token)
	if err != nil {
		t.Fatalf("ParseUser failed: %v", err)
	}
	if claims.UserID != 123 {
		t.Errorf("Expected UserID 123, got %d", claims.UserID)
	}
	if claims.Username != "admin" {
		t.Errorf("Expected Username 'admin', got %q", claims.Username)
	}
	if claims.Issuer != "ebook-server" {
		t.Errorf("Expected Issuer 'ebook-server', got %q", claims.Issuer)
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		t.Error("ExpiresAt and IssuedAt should both be set")
	}
}

func TestTokenBox_ParseRejectsGarbage(t *testing.T) {
	if _, err := newTestBox("test-secret-key", 1).ParseUser("invalid-token"); err == nil {
		t.Error("Expected error for invalid token")
	}
}

// TestTokenBox_CrossSecretRejected 两把密钥的两个实例互不认账（ADR-0001/0010）。
func TestTokenBox_CrossSecretRejected(t *testing.T) {
	token, err := newTestBox("secret-a", 60).IssueUser(1, "testuser")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}
	if _, err := newTestBox("secret-b", 60).ParseUser(token); err == nil {
		t.Error("Expected error for token signed with another secret")
	}
}

// TestTokenBox_RejectsExpired 过期 token 必须被拒。
func TestTokenBox_RejectsExpired(t *testing.T) {
	box := New("test-secret", 0) // 0 分钟 → 立即过期

	token, err := box.IssueUser(1, "testuser")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if _, err := box.ParseUser(token); err == nil {
		t.Error("Expected error for expired token")
	}
}

// TestTokenBox_Expire 构造参数原样成为有效期，包括 0。
//
// New 的注释承诺「不在这里偷偷兜底成默认值」，因为默认值属配置层的职责；这条就是那句
// 承诺的断言：配 0 分钟就回 0，而不是悄悄变成 60。后台 Session 经 Expire() 取这个值
// 拼 RegisteredClaims（它自己存一份就会有两个真相来源），所以这里也得是真值。
func TestTokenBox_Expire(t *testing.T) {
	if got := New("s", 45).Expire(); got != 45*time.Minute {
		t.Errorf("Expire 没有反映构造参数: got %v want 45m", got)
	}
	if got := New("s", 0).Expire(); got != 0 {
		t.Errorf("0 分钟被偷偷兜底成了 %v：默认值该由 config 层给", got)
	}
}

// TestTokenBox_RejectsForeignAlgorithm 算法白名单真的在挡事。
//
// JWT 头部是 base64url 编码的，明文里不含 "HS256" 字面量，所以「把头部里的 HS256
// 换成 none」这种写法是空操作、测不到任何控制。这里自己编码要用的头部。
func TestTokenBox_RejectsForeignAlgorithm(t *testing.T) {
	const secret = "test-secret"
	box := New(secret, 60)

	token, err := box.IssueUser(1, "someone")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 token parts, got %d", len(parts))
	}

	t.Run("alg=none 必须被拒", func(t *testing.T) {
		noneHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
		forged := noneHeader + "." + parts[1] + "."
		if err := box.Parse(forged, &Claims{}); err == nil {
			t.Error("Expected alg=none token to be rejected")
		}
	})

	t.Run("同密钥的 HS512 签名也必须被拒", func(t *testing.T) {
		// 只接受 HS256：算法是 token 头部自带的、可被调用方选择的参数，
		// 不钉住就等于让来客挑验签算法。
		now := time.Now()
		other, err := jwt.NewWithClaims(jwt.SigningMethodHS512, &Claims{
			UserID:   1,
			Username: "someone",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(now),
				Issuer:    "ebook-server",
			},
		}).SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("sign HS512 sample failed: %v", err)
		}
		if err := box.Parse(other, &Claims{}); err == nil {
			t.Error("Expected HS512-signed token to be rejected by the algorithm whitelist")
		}
	})
}

// TestTokenBox_RejectsMissingExpiry 没有 exp 的 token 必须被拒。
//
// golang-jwt v5 的过期校验是「声明里有 exp 才校验」：一枚不带 exp、但签名正确的
// token 会因为没有任何东西可过期而永远有效。签发方当然会写 exp，这条守的是
// 密钥泄露或被离线构造之后的下限——也正是 Parse 注释里承诺「过期」的一部分。
func TestTokenBox_RejectsMissingExpiry(t *testing.T) {
	box := New("test-secret", 60)

	noExp := &Claims{
		UserID:   1,
		Username: "someone",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
			Issuer:   Issuer,
		},
	}
	token, err := box.Issue(noExp)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if err := box.Parse(token, &Claims{}); err == nil {
		t.Error("Expected token without exp to be rejected")
	}

	// 同一份声明补上 exp 就应当通过：确认被拒的是「缺过期」而不是别的规则。
	withExp := &Claims{
		UserID:   1,
		Username: "someone",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Issuer:    Issuer,
		},
	}
	good, err := box.Issue(withExp)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if err := box.Parse(good, &Claims{}); err != nil {
		t.Errorf("Parse of a complete token failed: %v", err)
	}
}

// TestTokenBox_RejectsForeignIssuer 签发者不符的 token 必须被拒。
//
// 两端共用这一份 Parse，Issuer 是它们同值的唯一来源（pkg/jwt.Issuer）。
// 不校验它，类型注释里「算法、签发者、过期与验签规则只有一份实现」就只是措辞。
func TestTokenBox_RejectsForeignIssuer(t *testing.T) {
	box := New("test-secret", 60)

	wrong, err := box.Issue(&Claims{
		UserID:   1,
		Username: "someone",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Issuer:    "some-other-service",
		},
	})
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if err := box.Parse(wrong, &Claims{}); err == nil {
		t.Error("Expected token with a foreign iss to be rejected")
	}
}
