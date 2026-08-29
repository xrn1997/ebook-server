package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"ebook-server/middleware"
)

func TestCommentHandler_GetList_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.GET("/api/comments", commentHandler.GetList)

	_, token := registerUser(t, router, "list@example.com")

	for i := 0; i < 3; i++ {
		commentBody := map[string]string{"content": "Comment " + strconv.Itoa(i)}
		jsonBody, _ := json.Marshal(commentBody)
		req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	req, _ := http.NewRequest("GET", "/api/comments?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	decodeData(t, w.Body.Bytes())
}

func TestCommentHandler_GetList_Empty(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	commentHandler := NewCommentHandler()
	router.GET("/api/comments", commentHandler.GetList)

	req, _ := http.NewRequest("GET", "/api/comments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
}

func TestCommentHandler_Create_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)

	_, token := registerUser(t, router, "create@example.com")

	commentBody := map[string]string{"content": "This is a test comment"}
	jsonBody, _ := json.Marshal(commentBody)
	req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	decodeData(t, w.Body.Bytes())
}

func TestCommentHandler_Create_NoAuth(t *testing.T) {
	router := setupRouter()
	commentHandler := NewCommentHandler()
	router.POST("/api/comments", commentHandler.Create)

	body := map[string]string{"content": "This is a comment"}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestCommentHandler_Create_EmptyContent(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)

	_, token := registerUser(t, router, "empty@example.com")

	commentBody := map[string]string{"content": ""}
	jsonBody, _ := json.Marshal(commentBody)
	req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestCommentHandler_Delete_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.DELETE("/api/comments/:id", middleware.JWTAuth(), commentHandler.Delete)

	_, token := registerUser(t, router, "delete@example.com")

	commentBody := map[string]string{"content": "Comment to delete"}
	jsonBody, _ := json.Marshal(commentBody)
	req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	data := decodeData(t, w.Body.Bytes())
	commentID := uint(data["id"].(float64))

	req, _ = http.NewRequest("DELETE", "/api/comments/"+strconv.Itoa(int(commentID)), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
}

func TestCommentHandler_Delete_NoAuth(t *testing.T) {
	router := setupRouter()
	commentHandler := NewCommentHandler()
	router.DELETE("/api/comments/:id", commentHandler.Delete)

	req, _ := http.NewRequest("DELETE", "/api/comments/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestCommentHandler_Delete_InvalidID(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.DELETE("/api/comments/:id", middleware.JWTAuth(), commentHandler.Delete)

	_, token := registerUser(t, router, "invalid@example.com")

	req, _ := http.NewRequest("DELETE", "/api/comments/invalid", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestCommentHandler_Delete_NoPermission(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.DELETE("/api/comments/:id", middleware.JWTAuth(), commentHandler.Delete)

	_, token1 := registerUser(t, router, "user1@example.com")
	_, token2 := registerUser(t, router, "user2@example.com")

	// user1 创建评论
	commentBody := map[string]string{"content": "Comment by user1"}
	jsonBody, _ := json.Marshal(commentBody)
	req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token1)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	data := decodeData(t, w.Body.Bytes())
	commentID := uint(data["id"].(float64))

	// user2 尝试删除 user1 的评论
	req, _ = http.NewRequest("DELETE", "/api/comments/"+strconv.Itoa(int(commentID)), nil)
	req.Header.Set("Authorization", "Bearer "+token2)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0403")
}

func TestCommentHandler_GetMyComments_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.GET("/api/comments/my", middleware.JWTAuth(), commentHandler.GetMyComments)

	_, token := registerUser(t, router, "mycomments@example.com")

	commentBody := map[string]string{"content": "My comment"}
	jsonBody, _ := json.Marshal(commentBody)
	req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	req, _ = http.NewRequest("GET", "/api/comments/my", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 1 {
		t.Errorf("Expected 1 comment, got %d", len(items))
	}
}

func TestCommentHandler_GetMyComments_NoAuth(t *testing.T) {
	router := setupRouter()
	commentHandler := NewCommentHandler()
	router.GET("/api/comments/my", middleware.JWTAuth(), commentHandler.GetMyComments)

	req, _ := http.NewRequest("GET", "/api/comments/my", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestCommentHandler_GetList_Pagination(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.GET("/api/comments", commentHandler.GetList)

	_, token := registerUser(t, router, "paginate@example.com")

	for i := 0; i < 15; i++ {
		commentBody := map[string]string{"content": "Comment " + strconv.Itoa(i)}
		jsonBody, _ := json.Marshal(commentBody)
		req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	req, _ := http.NewRequest("GET", "/api/comments?page=1&page_size=5", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 5 {
		t.Errorf("Expected 5 items on page 1, got %d", len(items))
	}
	if data["page"].(float64) != 1 {
		t.Errorf("Expected page 1, got %v", data["page"])
	}

	req, _ = http.NewRequest("GET", "/api/comments?page=2&page_size=5", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	data = decodeData(t, w.Body.Bytes())
	items = data["items"].([]interface{})
	if len(items) != 5 {
		t.Errorf("Expected 5 items on page 2, got %d", len(items))
	}
}

func TestCommentHandler_GetMyComments_Pagination(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.GET("/api/comments/my", middleware.JWTAuth(), commentHandler.GetMyComments)

	_, token := registerUser(t, router, "mypaginate@example.com")

	for i := 0; i < 5; i++ {
		commentBody := map[string]string{"content": "Comment " + strconv.Itoa(i)}
		jsonBody, _ := json.Marshal(commentBody)
		req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	req, _ := http.NewRequest("GET", "/api/comments/my?page=1&page_size=3", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}
	if data["total"].(float64) != 5 {
		t.Errorf("Expected total 5, got %v", data["total"])
	}
}

func TestCommentHandler_Create_InvalidJSON(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)

	_, token := registerUser(t, router, "invalidjson@example.com")

	req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestCommentHandler_Delete_NotFound(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.DELETE("/api/comments/:id", middleware.JWTAuth(), commentHandler.Delete)

	_, token := registerUser(t, router, "notfound@example.com")

	req, _ := http.NewRequest("DELETE", "/api/comments/999999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0404")
}
