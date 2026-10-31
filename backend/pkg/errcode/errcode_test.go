package errcode

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ebook-server/model"

	"github.com/gin-gonic/gin"
)

func setupErrcodeContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	return c, w
}

func TestSuccess(t *testing.T) {
	c, w := setupErrcodeContext()
	Success(c, gin.H{"name": "test"})

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["code"] != SuccessCode {
		t.Errorf("Expected code '%s', got %v", SuccessCode, resp["code"])
	}
	if resp["error"] != "" {
		t.Errorf("Expected empty error, got %v", resp["error"])
	}
	data := resp["data"].(map[string]interface{})
	if data["name"] != "test" {
		t.Errorf("Expected data.name 'test', got %v", data["name"])
	}
}

func TestSuccess_NilData(t *testing.T) {
	c, w := setupErrcodeContext()
	Success(c, nil)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["code"] != SuccessCode {
		t.Errorf("Expected code '%s', got %v", SuccessCode, resp["code"])
	}
	// data 为 nil 时应省略（omitempty）
	if _, exists := resp["data"]; exists {
		t.Error("Expected data to be omitted when nil")
	}
}

func TestError(t *testing.T) {
	c, w := setupErrcodeContext()
	Error(c, BadRequest, "参数错误")

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["code"] != BadRequest {
		t.Errorf("Expected code '%s', got %v", BadRequest, resp["code"])
	}
	if resp["error"] != "参数错误" {
		t.Errorf("Expected error '参数错误', got %v", resp["error"])
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected HTTP 200, got %d", w.Code)
	}
}

func TestSuccessMsg(t *testing.T) {
	c, w := setupErrcodeContext()
	SuccessMsg(c, "操作成功", gin.H{"id": 1})

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["code"] != SuccessCode {
		t.Errorf("Expected code '%s', got %v", SuccessCode, resp["code"])
	}
	if resp["error"] != "操作成功" {
		t.Errorf("Expected error '操作成功', got %v", resp["error"])
	}
	data := resp["data"].(map[string]interface{})
	if data["id"].(float64) != 1 {
		t.Errorf("Expected data.id 1, got %v", data["id"])
	}
}

func TestError_Codes(t *testing.T) {
	// 验证各业务码常量非空
	codes := map[string]string{
		SuccessCode:           "00000",
		UsernameInvalid:       "A0110",
		UsernameExists:        "A0111",
		EmailExists:           "A0112",
		EmailImmutable:        "A0113",
		PasswordInvalid:       "A0120",
		CodeInvalid:           "A0132",
		EmailInvalid:          "A0153",
		AccountNotFound:       "A0201",
		PasswordWrong:         "A0210",
		LoginExpired:          "A0230",
		TokenInvalid:          "A0240",
		AttemptTooMany:        "A0241",
		AccountLocked:         "A0242",
		MailSendFailed:        "C0503",
		CommentContentEmpty:   "A0301",
		CommentChapterInvalid: "A0302",
		CommentNotOwner:       "A0303",
		CommentNotFoundErr:    "A0304",
		CommentKeySame:        "A0305",
		BadRequest:            "A0400",
		NotFound:              "A0404",
		Forbidden:             "A0403",
		ServerError:           "C0500",
	}

	for name, expected := range codes {
		if name != expected {
			t.Errorf("Code constant mismatch: key=%s, value=%s", name, expected)
		}
	}
}

// TestFrom_ResolvesThroughWrappedErrors 业务码沿包装链查找。
//
// 表原来是 `errorCodes[err]` 的同一性查找，等于在每条 service 接口上偷偷写一条
// 「领域错误不许用 %w 包装」的约束——破了的静默落 C0500，且没有任何提示。
func TestFrom_ResolvesThroughWrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("重置密码失败：%w", model.ErrCommentKeySame)
	if got := From(wrapped); got != CommentKeySame {
		t.Errorf("From(wrapped) = %s, want %s", got, CommentKeySame)
	}
	deep := fmt.Errorf("外层上下文：%w", wrapped)
	if got := From(deep); got != CommentKeySame {
		t.Errorf("From(twice-wrapped) = %s, want %s", got, CommentKeySame)
	}
}

// unhashableError 模拟 validator.ValidationErrors 这类不可作 map key 的 error。
type unhashableError []string

func (e unhashableError) Error() string { return "unhashable" }

// TestFrom_UnhashableErrorDoesNotPanic 未经登记的任意 error 都必须安全落 C0500。
//
// 旧的 map[error]string 查表会对不可哈希的动态类型 panic；错误映射是兜底路径，
// 不能因为错误本身的形状把服务器打崩。
func TestFrom_UnhashableErrorDoesNotPanic(t *testing.T) {
	if got := From(unhashableError{"x"}); got != ServerError {
		t.Fatalf("From(unhashable) = %s, want %s", got, ServerError)
	}
}

// TestRespond_OverridesApplyPerCallSite 端点级覆盖写在调用点，而不是各 handler 抄一份 switch。
func TestRespond_OverridesApplyPerCallSite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("覆盖命中即用覆盖的码与文案", func(t *testing.T) {
		c, w := setupErrcodeContext()
		Respond(c, model.ErrAccountNotFound, "登录失败",
			Map(model.ErrAccountNotFound, PasswordWrong, "邮箱或密码错误"))
		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if resp["code"] != PasswordWrong || resp["error"] != "邮箱或密码错误" {
			t.Errorf("got %q/%q, want %q/%q", resp["code"], resp["error"], PasswordWrong, "邮箱或密码错误")
		}
	})

	t.Run("覆盖用 errors.Is 因此包装过的错误也能被挡住", func(t *testing.T) {
		c, w := setupErrcodeContext()
		Respond(c, fmt.Errorf("带上下文：%w", model.ErrAccountNotFound), "登录失败",
			Map(model.ErrAccountNotFound, PasswordWrong, "邮箱或密码错误"))
		var resp map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["code"] != PasswordWrong {
			t.Errorf("code = %q, want %q", resp["code"], PasswordWrong)
		}
	})

	t.Run("未命中覆盖则回落到默认表", func(t *testing.T) {
		c, w := setupErrcodeContext()
		Respond(c, model.ErrCommentNotOwner, "删除失败",
			Map(model.ErrAccountNotFound, PasswordWrong, "邮箱或密码错误"))
		var resp map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["code"] != CommentNotOwner {
			t.Errorf("code = %q, want %q", resp["code"], CommentNotOwner)
		}
	})
}

// TestRespond_UnregisteredErrorLeaksNothing 未登记的错误只回兜底文案。
//
// From 现在沿包装链找码，因此「未登记 → 不透出 err.Error()」这条判断是整个
// 信封不泄露驱动/网络细节的唯一屏障，必须有直接盯着它的用例。
func TestRespond_UnregisteredErrorLeaksNothing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, w := setupErrcodeContext()

	detail := errors.New("dial tcp 10.0.0.1:3306: connect refused (user=root)")
	Respond(c, detail, "查询用户失败")

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp["code"] != ServerError {
		t.Errorf("code = %s, want %s", resp["code"], ServerError)
	}
	if resp["error"] != "查询用户失败" {
		t.Errorf("error = %q, want the fallback text only", resp["error"])
	}
	for _, leak := range []string{"10.0.0.1", "root", "tcp"} {
		if strings.Contains(resp["error"], leak) {
			t.Errorf("response leaked %q: %s", leak, resp["error"])
		}
	}
}

// TestRespond_RegisteredChainSurfacesReason 登记过的错误沿链命中，透出包装后的理由。
//
// 这条是约定的另一面：service 用 %w 附上的理由文本本身就是给用户看的（如
// 「聚合键不合规：目标聚合键不能为空」），所以文案取自最外层错误。
func TestRespond_RegisteredChainSurfacesReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, w := setupErrcodeContext()

	Respond(c, fmt.Errorf("%w：目标聚合键不能为空", model.ErrCommentKeyInvalid), "改键失败")

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp["code"] != BadRequest {
		t.Errorf("code = %s, want %s", resp["code"], BadRequest)
	}
	if !strings.Contains(resp["error"], "目标聚合键不能为空") {
		t.Errorf("error = %q, want it to carry the wrapped reason", resp["error"])
	}
}
