package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDGenerated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": GetRequestID(c)})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	id := rec.Header().Get(RequestIDHeader)
	if id == "" {
		t.Fatal("missing response request id")
	}
	var body struct{ ID string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != id {
		t.Fatalf("context id %q != header %q", body.ID, id)
	}
}

func TestRequestIDUsesValidClientValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, GetRequestID(c)) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "client-123")
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get(RequestIDHeader); got != "client-123" {
		t.Fatalf("header = %q, want client-123", got)
	}
	if got := rec.Body.String(); got != "client-123" {
		t.Fatalf("body = %q, want client-123", got)
	}
}

func TestRequestIDRejectsUnsafeClientValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, GetRequestID(c)) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "bad id!\n")
	r.ServeHTTP(rec, req)

	got := rec.Header().Get(RequestIDHeader)
	if got == "bad id!\n" || got == "" {
		t.Fatalf("unsafe request id was not replaced: %q", got)
	}
}

func TestRequestIDIsUniquePerRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, GetRequestID(c)) })

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		id := rec.Header().Get(RequestIDHeader)
		if id == "" || seen[id] {
			t.Fatalf("request %d got id %q", i, id)
		}
		seen[id] = true
	}
}
