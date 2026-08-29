package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"ebook-server/middleware"
	"ebook-server/pkg/jwt"
)

func TestCommentHandler_GetList_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	// 创建一些评论
	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.GET("/api/comments", commentHandler.GetList)

	// 注册用户
	registerBody := map[string]string{
		"username": "listcommentuser",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))
	token, _ := jwt.GenerateToken(userID, "listcommentuser")

	// 创建评论
	for i := 0; i < 3; i++ {
		commentBody := map[string]string{
			"content": "Comment " + strconv.Itoa(i),
		}
		jsonBody, _ = json.Marshal(commentBody)
		req, _ = http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// 获取评论列表
	req, _ = http.NewRequest("GET", "/api/comments?page=1&page_size=10", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["code"] != float64(200) {
		t.Errorf("Expected code 200, got %v", response["code"])
	}
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
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestCommentHandler_Create_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)

	// 注册
	registerBody := map[string]string{
		"username": "createcommentuser",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))
	token, _ := jwt.GenerateToken(userID, "createcommentuser")

	// 创建评论
	commentBody := map[string]string{
		"content": "This is a test comment",
	}
	jsonBody, _ = json.Marshal(commentBody)
	req, _ = http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["code"] != float64(200) {
		t.Errorf("Expected code 200, got %v", response["code"])
	}
}

func TestCommentHandler_Create_NoAuth(t *testing.T) {
	router := setupRouter()
	commentHandler := NewCommentHandler()
	router.POST("/api/comments", commentHandler.Create)

	body := map[string]string{
		"content": "This is a comment",
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestCommentHandler_Create_EmptyContent(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)

	// 注册
	registerBody := map[string]string{
		"username": "emptycontentuser",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))
	token, _ := jwt.GenerateToken(userID, "emptycontentuser")

	// 创建空内容评论
	commentBody := map[string]string{
		"content": "",
	}
	jsonBody, _ = json.Marshal(commentBody)
	req, _ = http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestCommentHandler_Delete_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.DELETE("/api/comments/:id", middleware.JWTAuth(), commentHandler.Delete)

	// 注册
	registerBody := map[string]string{
		"username": "deletecommentuser",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))
	token, _ := jwt.GenerateToken(userID, "deletecommentuser")

	// 创建评论
	commentBody := map[string]string{
		"content": "Comment to delete",
	}
	jsonBody, _ = json.Marshal(commentBody)
	req, _ = http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	createData := createResp["data"].(map[string]interface{})
	commentID := uint(createData["id"].(float64))

	// 删除评论
	req, _ = http.NewRequest("DELETE", "/api/comments/"+strconv.Itoa(int(commentID)), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestCommentHandler_Delete_NoAuth(t *testing.T) {
	router := setupRouter()
	commentHandler := NewCommentHandler()
	router.DELETE("/api/comments/:id", commentHandler.Delete)

	req, _ := http.NewRequest("DELETE", "/api/comments/1", nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestCommentHandler_Delete_InvalidID(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.DELETE("/api/comments/:id", middleware.JWTAuth(), commentHandler.Delete)

	// 注册
	registerBody := map[string]string{
		"username": "invalididuser",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))
	token, _ := jwt.GenerateToken(userID, "invalididuser")

	// 删除无效 ID
	req, _ = http.NewRequest("DELETE", "/api/comments/invalid", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestCommentHandler_Delete_NoPermission(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.DELETE("/api/comments/:id", middleware.JWTAuth(), commentHandler.Delete)

	// 注册 user1
	registerBody1 := map[string]string{
		"username": "user1_noperm",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody1)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var registerResp1 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp1)
	data1 := registerResp1["data"].(map[string]interface{})
	userID1 := uint(data1["id"].(float64))
	token1, _ := jwt.GenerateToken(userID1, "user1_noperm")

	// 注册 user2
	registerBody2 := map[string]string{
		"username": "user2_noperm",
		"password": "password123",
	}
	jsonBody, _ = json.Marshal(registerBody2)
	req, _ = http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var registerResp2 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp2)
	data2 := registerResp2["data"].(map[string]interface{})
	userID2 := uint(data2["id"].(float64))
	token2, _ := jwt.GenerateToken(userID2, "user2_noperm")

	// user1 创建评论
	commentBody := map[string]string{
		"content": "Comment by user1",
	}
	jsonBody, _ = json.Marshal(commentBody)
	req, _ = http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token1)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	createData := createResp["data"].(map[string]interface{})
	commentID := uint(createData["id"].(float64))

	// user2 尝试删除 user1 的评论
	req, _ = http.NewRequest("DELETE", "/api/comments/"+strconv.Itoa(int(commentID)), nil)
	req.Header.Set("Authorization", "Bearer "+token2)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", w.Code)
	}
}

func TestCommentHandler_GetMyComments_Success(t *testing.T) {
	setupHandlerTestDB(t)
	defer cleanupHandlerTestDB(t)

	router := setupRouter()
	authHandler := NewAuthHandler()
	commentHandler := NewCommentHandler()
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.GET("/api/comments/my", middleware.JWTAuth(), commentHandler.GetMyComments)

	// 注册
	registerBody := map[string]string{
		"username": "mycommentsuser",
		"password": "password123",
	}
	jsonBody, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var registerResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &registerResp)
	data := registerResp["data"].(map[string]interface{})
	userID := uint(data["id"].(float64))
	token, _ := jwt.GenerateToken(userID, "mycommentsuser")

	// 创建评论
	commentBody := map[string]string{
		"content": "My comment",
	}
	jsonBody, _ = json.Marshal(commentBody)
	req, _ = http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 获取我的评论
	req, _ = http.NewRequest("GET", "/api/comments/my", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	result := response["data"].(map[string]interface{})
	items := result["items"].([]interface{})

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

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}
