package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ebook-server/pkg/jwt"

	"github.com/gin-gonic/gin"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// testTokens 测试用的 token 单元（密钥与有效期在构造时给定，无全局配置）。
func testTokens(expireMin int) *jwt.TokenBox {
	return jwt.New("test-secret", expireMin)
}

// denyParser 永远拒绝的 Parser adapter。
//
// 它是这个 seam 上的第二个 adapter（另一个是真 TokenBox）：有了它，「未登录被挡」
// 才真的是在测中间件挂了没挂，而不是恰好因为 token 有效而通过。
type denyParser struct{}

func (denyParser) ParseUser(string) (*jwt.Claims, error) {
	return nil, errors.New("denied")
}

// protected 造一个只有「中间件 + 回显身份」的路由。
func protected(tokens Parser) *gin.Engine {
	r := setupTestRouter()
	r.GET("/protected", JWTAuth(tokens), func(c *gin.Context) {
		userID, _ := GetCurrentUserID(c)
		username, _ := GetCurrentUsername(c)
		c.JSON(http.StatusOK, gin.H{"user_id": userID, "username": username})
	})
	return r
}

// doAuth 带 Authorization 头请求 /protected；header 传空串表示匿名。
func doAuth(r *gin.Engine, header string) map[string]interface{} {
	req, _ := http.NewRequest("GET", "/protected", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp
}

func TestJWTAuth_ValidToken(t *testing.T) {
	tokens := testTokens(1)
	token, err := tokens.IssueUser(42, "testuser")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}

	resp := doAuth(protected(tokens), "Bearer "+token)
	if resp["user_id"].(float64) != 42 {
		t.Errorf("Expected user_id 42, got %v", resp["user_id"])
	}
	if resp["username"] != "testuser" {
		t.Errorf("Expected username 'testuser', got %v", resp["username"])
	}
}

func TestJWTAuth_Rejects(t *testing.T) {
	tokens := testTokens(1)
	valid, err := tokens.IssueUser(1, "testuser")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}

	expiredBox := testTokens(0)
	expired, err := expiredBox.IssueUser(1, "testuser")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	cases := []struct {
		name   string
		parser Parser
		header string
	}{
		{"缺少认证头", tokens, ""},
		{"认证格式错误", tokens, "NotBearer token"},
		{"token 无法解析", tokens, "Bearer invalid-token-string"},
		{"token 已过期", expiredBox, "Bearer " + expired},
		// 同一份声明、另一把密钥：后台 token 打不进公开 API（ADR-0001/0010）
		{"密钥不匹配", jwt.New("another-secret", 1), "Bearer " + valid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if resp := doAuth(protected(tc.parser), tc.header); resp["code"] != "A0230" {
				t.Errorf("Expected code A0230, got %v", resp["code"])
			}
		})
	}
}

// TestJWTAuth_RunsParser 中间件确实把 token 交给解析器并据其结果放行/拦截。
func TestJWTAuth_RunsParser(t *testing.T) {
	if resp := doAuth(protected(denyParser{}), "Bearer anything"); resp["code"] != "A0230" {
		t.Errorf("Expected A0230 when the parser denies everything, got %v", resp["code"])
	}
}

func TestGetCurrentUserID_Accessors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if _, exists := GetCurrentUserID(c); exists {
		t.Error("Expected GetCurrentUserID to return false when not set")
	}
	if _, exists := GetCurrentUsername(c); exists {
		t.Error("Expected GetCurrentUsername to return false when not set")
	}

	c.Set("user_id", uint(42))
	c.Set("username", "testuser")
	if id, exists := GetCurrentUserID(c); !exists || id != 42 {
		t.Errorf("Expected user_id 42, got %d (exists=%v)", id, exists)
	}
	if name, exists := GetCurrentUsername(c); !exists || name != "testuser" {
		t.Errorf("Expected username 'testuser', got %q (exists=%v)", name, exists)
	}
}

// TestJWTAuth_RejectsTokenNamingNoAccount 不指名账号的 token 不算已认证。
//
// uid 0 在数据层是「不限用户」的形态；中间件放它过去，下游一切「只动本人行」的
// 操作都会变成跨用户读写。这里当场挡掉，让 handler.CurrentUser 的契约成立。
func TestJWTAuth_RejectsTokenNamingNoAccount(t *testing.T) {
	box := jwt.New("test-secret", 30)
	token, err := box.IssueUser(0, "ghost")
	if err != nil {
		t.Fatalf("IssueUser failed: %v", err)
	}

	if got := doAuth(protected(box), "Bearer "+token)["code"]; got != "A0230" {
		t.Errorf("code = %v, want A0230", got)
	}
}
