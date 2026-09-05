package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"ebook-server/middleware"
)

func TestCommentHandler_GetList_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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

// TestCommentHandler_Create_WithChapter 创建章节评论并校验响应视图契约（ADR-0011）：
// user 只含 uid/username/nickname/avatar 四字段（无 email），add_time 为格式化字符串。
func TestCommentHandler_Create_WithChapter(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)

	_, token := registerUser(t, router, "chapter_create@example.com")

	body := map[string]interface{}{
		"content":      "章节评论内容",
		"chapter_url":  "https://src.example.com/book/1/2.html",
		"chapter_name": "第二章",
		"book_name":    "天启之书",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	if data["chapter_url"] != body["chapter_url"] || data["book_name"] != body["book_name"] {
		t.Errorf("chapter fields missing in response: %v", data)
	}
	user := data["user"].(map[string]interface{})
	for _, key := range []string{"uid", "username", "nickname", "avatar"} {
		if _, ok := user[key]; !ok {
			t.Errorf("user view missing key %q: %v", key, user)
		}
	}
	if _, leaked := user["email"]; leaked {
		t.Error("user view must not leak email")
	}
	if _, ok := data["add_time"]; !ok {
		t.Error("add_time missing in response")
	}
}

// TestCommentHandler_GetList_ByChapter 按 chapter_url 过滤章节评论（ADR-0011）。
func TestCommentHandler_GetList_ByChapter(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.GET("/api/comments", commentHandler.GetList)

	_, token := registerUser(t, router, "chapter_list_h@example.com")

	urlA := "https://src.example.com/book/1/2.html"
	create := func(m map[string]string) {
		jsonBody, _ := json.Marshal(m)
		req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
	create(map[string]string{"content": "A1", "chapter_url": urlA})
	create(map[string]string{"content": "A2", "chapter_url": urlA})
	create(map[string]string{"content": "通用"})

	// 按章节过滤：只返回该章节的 2 条
	req, _ := http.NewRequest("GET", "/api/comments?chapter_url="+url.QueryEscape(urlA), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "00000" {
		t.Fatalf("list by chapter failed: %v", resp)
	}
	data := resp["data"].(map[string]interface{})
	if data["total"].(float64) != 2 {
		t.Errorf("Expected total 2 for chapter filter, got %v", data["total"])
	}
}

func TestCommentHandler_GetList_Empty(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	commentHandler := app.comment
	router.GET("/api/comments", commentHandler.GetList)

	req, _ := http.NewRequest("GET", "/api/comments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
}

func TestCommentHandler_Create_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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
	app := newTestApp(t)
	router := setupRouter()
	commentHandler := app.comment
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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
	app := newTestApp(t)
	router := setupRouter()
	commentHandler := app.comment
	router.DELETE("/api/comments/:id", commentHandler.Delete)

	req, _ := http.NewRequest("DELETE", "/api/comments/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestCommentHandler_Delete_InvalidID(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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

	// user2 尝试删除 user1 的评论（ADR-0011：仅本人可删，专用码 A0303）
	req, _ = http.NewRequest("DELETE", "/api/comments/"+strconv.Itoa(int(commentID)), nil)
	req.Header.Set("Authorization", "Bearer "+token2)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0303")
}

func TestCommentHandler_GetMyComments_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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
	app := newTestApp(t)
	router := setupRouter()
	commentHandler := app.comment
	router.GET("/api/comments/my", middleware.JWTAuth(), commentHandler.GetMyComments)

	req, _ := http.NewRequest("GET", "/api/comments/my", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestCommentHandler_GetList_Pagination(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
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
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.DELETE("/api/comments/:id", middleware.JWTAuth(), commentHandler.Delete)

	_, token := registerUser(t, router, "notfound@example.com")

	req, _ := http.NewRequest("DELETE", "/api/comments/999999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 评论不存在 → 评论域专用码 A0304（ADR-0011）
	assertErrorCode(t, w.Body.Bytes(), "A0304")
}

// TestCommentHandler_GetList_MultiChapterURL 多 chapter_url 参数返回并集。
func TestCommentHandler_GetList_MultiChapterURL(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.GET("/api/comments", commentHandler.GetList)

	_, token := registerUser(t, router, "multi_ch@example.com")

	create := func(m map[string]string) {
		jsonBody, _ := json.Marshal(m)
		req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
	create(map[string]string{"content": "A1", "chapter_url": "key-a"})
	create(map[string]string{"content": "A2", "chapter_url": "key-a"})
	create(map[string]string{"content": "B1", "chapter_url": "key-b"})
	create(map[string]string{"content": "C1", "chapter_url": "key-c"})

	// 多键并集：key-a + key-b = 3
	req, _ := http.NewRequest("GET", "/api/comments?chapter_url=key-a&chapter_url=key-b", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "00000" {
		t.Fatalf("multi-key query failed: %v", resp)
	}
	data := resp["data"].(map[string]interface{})
	if data["total"].(float64) != 3 {
		t.Errorf("Expected total 3, got %v", data["total"])
	}
}

// TestCommentHandler_GetList_SingleChapterURL_BackwardCompat 单 chapter_url 仍走原路径。
func TestCommentHandler_GetList_SingleChapterURL_BackwardCompat(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.GET("/api/comments", commentHandler.GetList)

	_, token := registerUser(t, router, "single_ch@example.com")

	create := func(m map[string]string) {
		jsonBody, _ := json.Marshal(m)
		req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		httptest.NewRecorder()
		router.ServeHTTP(httptest.NewRecorder(), req)
	}
	create(map[string]string{"content": "A1", "chapter_url": "key-a"})
	create(map[string]string{"content": "A2", "chapter_url": "key-a"})

	req, _ := http.NewRequest("GET", "/api/comments?chapter_url=key-a", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["total"].(float64) != 2 {
		t.Errorf("Expected total 2 for backward-compat single key, got %v", data["total"])
	}
}

func TestCommentHandler_MigrateKey_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.POST("/api/comments/migrate-key", middleware.JWTAuth(), commentHandler.MigrateKey)
	router.GET("/api/comments", commentHandler.GetList)

	_, token := registerUser(t, router, "migrate_h@example.com")

	// 创建两条旧键下的评论
	for _, c := range []string{"c1", "c2"} {
		body := map[string]string{"content": c, "chapter_url": "old-key"}
		jsonBody, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	// 迁移
	migBody, _ := json.Marshal(map[string]string{"old_key": "old-key", "new_key": "new-key"})
	req, _ := http.NewRequest("POST", "/api/comments/migrate-key", bytes.NewBuffer(migBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	if data["migrated_count"].(float64) != 2 {
		t.Errorf("Expected migrated_count 2, got %v", data["migrated_count"])
	}
}

func TestCommentHandler_MigrateKey_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentHandler := app.comment
	router.POST("/api/comments/migrate-key", commentHandler.MigrateKey)

	body, _ := json.Marshal(map[string]string{"old_key": "a", "new_key": "b"})
	req, _ := http.NewRequest("POST", "/api/comments/migrate-key", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestCommentHandler_MigrateKey_SameKey(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments/migrate-key", middleware.JWTAuth(), commentHandler.MigrateKey)

	_, token := registerUser(t, router, "samekey@example.com")

	body, _ := json.Marshal(map[string]string{"old_key": "same", "new_key": "same"})
	req, _ := http.NewRequest("POST", "/api/comments/migrate-key", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0305")
}

func TestCommentHandler_MigrateKey_EmptyParams(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments/migrate-key", middleware.JWTAuth(), commentHandler.MigrateKey)

	_, token := registerUser(t, router, "empty_mig@example.com")

	body, _ := json.Marshal(map[string]string{"old_key": "", "new_key": ""})
	req, _ := http.NewRequest("POST", "/api/comments/migrate-key", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestCommentHandler_MigrateKey_UserIsolation(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	commentHandler := app.comment
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/comments", middleware.JWTAuth(), commentHandler.Create)
	router.POST("/api/comments/migrate-key", middleware.JWTAuth(), commentHandler.MigrateKey)
	router.GET("/api/comments", commentHandler.GetList)

	_, token1 := registerUser(t, router, "iso_u1@example.com")
	_, token2 := registerUser(t, router, "iso_u2@example.com")

	// user1 和 user2 各创建一条同键评论
	for _, tk := range []string{token1, token2} {
		body := map[string]string{"content": "c", "chapter_url": "shared-key"}
		jsonBody, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/api/comments", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tk)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	// user1 迁移
	migBody, _ := json.Marshal(map[string]string{"old_key": "shared-key", "new_key": "new-key"})
	req, _ := http.NewRequest("POST", "/api/comments/migrate-key", bytes.NewBuffer(migBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token1)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	if data["migrated_count"].(float64) != 1 {
		t.Errorf("Expected 1 migrated (user isolation), got %v", data["migrated_count"])
	}
}
