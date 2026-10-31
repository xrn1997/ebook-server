package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestRecovery_PanicRecovered(t *testing.T) {
	// 初始化 zap 以避免 nil pointer
	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	req, _ := http.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Recovery 应捕获 panic 并返回 200 + ServerError 业务码
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 after recovery, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "C0500" {
		t.Errorf("Expected code C0500, got %v", resp["code"])
	}
}

// TestRecovery_DoesNotAppendEnvelopeAfterPartialWrite 响应写到一半才 panic 时，
// 不能再追加一个 C0500 信封：两段 JSON 拼在一起会让客户端解析失败，审计也提取
// 不出业务码。此时只保留已经写出的内容与 panic 日志。
func TestRecovery_DoesNotAppendEnvelopeAfterPartialWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/partial", func(c *gin.Context) {
		c.String(http.StatusOK, `{"code":"00000"`)
		panic("boom after write")
	})

	req, _ := http.NewRequest("GET", "/partial", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if got := w.Body.String(); got != `{"code":"00000"` {
		t.Errorf("Recovery must not append a second envelope, got %q", got)
	}
}

func TestRecovery_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/ok", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("Expected ok=true")
	}
}
