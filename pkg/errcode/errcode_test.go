package errcode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
		SuccessCode:     "00000",
		UsernameInvalid: "A0110",
		UsernameExists:  "A0111",
		EmailExists:     "A0112",
		PasswordInvalid: "A0120",
		CodeInvalid:     "A0132",
		EmailInvalid:    "A0153",
		AccountNotFound: "A0201",
		PasswordWrong:   "A0210",
		LoginExpired:    "A0230",
		TokenInvalid:    "A0240",
		AttemptTooMany:  "A0241",
		AccountLocked:   "A0242",
		MailSendFailed:  "C0503",
		BadRequest:      "A0400",
		NotFound:        "A0404",
		Forbidden:       "A0403",
		ServerError:     "C0500",
	}

	for name, expected := range codes {
		if name != expected {
			t.Errorf("Code constant mismatch: key=%s, value=%s", name, expected)
		}
	}
}
