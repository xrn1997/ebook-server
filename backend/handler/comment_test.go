package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"ebook-server/middleware"
	"ebook-server/model"

	"github.com/gin-gonic/gin"
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

// TestCommentHandler_GetList_MultiChapterURL 多 chapter_url 参数返回并集（合并书籍场景）。
func TestCommentHandler_GetList_MultiChapterURL(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	_, token := registerUser(t, router, "multi_ch@example.com")
	createComment(t, router, token, "A1", "key-a")
	createComment(t, router, token, "A2", "key-a")
	createComment(t, router, token, "B1", "key-b")
	createComment(t, router, token, "C1", "key-c")

	// 并集 = key-a 两条 + key-b 一条；key-c 不在查询内
	w := doJSON(t, router, http.MethodGet, "/api/comments?chapter_url=key-a&chapter_url=key-b", nil, "")
	data := decodeData(t, w.Body.Bytes())
	if data["total"].(float64) != 3 {
		t.Errorf("Expected union total 3, got %v", data["total"])
	}
}

// TestCommentHandler_GetList_SingleChapterURL 单键与多键共用一条 IN 路径（无等值分支）。
func TestCommentHandler_GetList_SingleChapterURL(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	_, token := registerUser(t, router, "single_ch@example.com")
	createComment(t, router, token, "A1", "key-a")
	createComment(t, router, token, "A2", "key-a")
	createComment(t, router, token, "B1", "key-b")

	if got := commentTotalUnder(t, router, "key-a"); got != 2 {
		t.Errorf("Expected total 2 for single key, got %v", got)
	}
}

// TestCommentHandler_GetList_EmptyChapterURLIgnored ?chapter_url= 不带值按未提供处理。
//
// 空串在聚合键语义里是「书籍级评论」这个键本身，但查询串里留空通常只是想过滤书名，
// 因此退化为不按章节过滤而不是返回空集。
func TestCommentHandler_GetList_EmptyChapterURLIgnored(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	_, token := registerUser(t, router, "empty_ch@example.com")
	createComment(t, router, token, "书籍级评论", "")

	w := doJSON(t, router, http.MethodGet, "/api/comments?chapter_url=", nil, "")
	data := decodeData(t, w.Body.Bytes())
	if data["total"].(float64) != 1 {
		t.Errorf("Expected empty filter to be ignored (total 1), got %v", data["total"])
	}
}

// TestCommentHandler_GetList_ChapterURLLimits 公开端点的聚合键入参两条上限都必须生效。
//
// 数量上限防的是「把 SQLite 单语句绑定变量打满」，长度上限防的是永远匹配不到的无效键；
// 越界与恰好在上限内两种情形都验，否则上限写错方向（off-by-one）不会被发现。
func TestCommentHandler_GetList_ChapterURLLimits(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	// 超出数量上限
	w := doJSON(t, router, http.MethodGet, "/api/comments?"+chapterURLQuery(maxChapterURLFilters+1), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0400")
	// 超出单键长度上限
	w = doJSON(t, router, http.MethodGet, "/api/comments?chapter_url="+strings.Repeat("a", maxChapterURLFilterLength+1), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0400")
	// 恰好在上限内不得误伤
	w = doJSON(t, router, http.MethodGet, "/api/comments?"+chapterURLQuery(maxChapterURLFilters), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "00000")
	w = doJSON(t, router, http.MethodGet, "/api/comments?chapter_url="+strings.Repeat("a", maxChapterURLFilterLength), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "00000")
}

// chapterURLQuery 拼出 n 个 chapter_url 参数的查询串（键名唯一，便于以后按内容断言）。
func chapterURLQuery(n int) string {
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf("chapter_url=k%d", i))
	}
	return strings.Join(parts, "&")
}

// commentTotalUnder 查某聚合键下的评论总数（走公开列表端点，匿名即可）。
func commentTotalUnder(t *testing.T, router *gin.Engine, key string) float64 {
	t.Helper()
	w := doJSON(t, router, http.MethodGet, "/api/comments?chapter_url="+url.QueryEscape(key), nil, "")
	data := decodeData(t, w.Body.Bytes())
	return data["total"].(float64)
}

// migrateKey 以 token 身份发一次聚合键迁移请求（token 传空串 = 匿名）。
func migrateKey(t *testing.T, router *gin.Engine, token, oldKey, newKey string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, router, http.MethodPost, "/api/comments/migrate-key", map[string]string{
		"old_key": oldKey,
		"new_key": newKey,
	}, token)
}

// TestCommentHandler_MigrateKey_Success 迁移后评论只在新键下可见，且重复迁移为幂等。
func TestCommentHandler_MigrateKey_Success(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	_, token := registerUser(t, router, "migrate_h@example.com")
	createComment(t, router, token, "c1", "old-key")
	createComment(t, router, token, "c2", "old-key")

	data := decodeData(t, migrateKey(t, router, token, "old-key", "new-key").Body.Bytes())
	if data["migrated_count"].(float64) != 2 {
		t.Fatalf("Expected migrated_count 2, got %v", data["migrated_count"])
	}
	if got := commentTotalUnder(t, router, "new-key"); got != 2 {
		t.Errorf("Expected 2 comments under new key, got %v", got)
	}
	if got := commentTotalUnder(t, router, "old-key"); got != 0 {
		t.Errorf("Expected old key emptied, got %v", got)
	}

	// 无匹配行返回 0 而非报错：合并流程会重试，报错会让客户端无法判断是否已生效
	data = decodeData(t, migrateKey(t, router, token, "old-key", "new-key").Body.Bytes())
	if data["migrated_count"].(float64) != 0 {
		t.Errorf("Expected idempotent retry to migrate 0, got %v", data["migrated_count"])
	}
}

// TestCommentHandler_MigrateKey_NoAuth 未登录不得改别人的聚合键。
func TestCommentHandler_MigrateKey_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	assertErrorCode(t, migrateKey(t, router, "", "a", "b").Body.Bytes(), "A0230")
}

// TestCommentHandler_MigrateKey_SameKey 新旧相同返回 A0305，而不是静默「迁移 0 行」。
func TestCommentHandler_MigrateKey_SameKey(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	_, token := registerUser(t, router, "samekey@example.com")

	assertErrorCode(t, migrateKey(t, router, token, "same", "same").Body.Bytes(), "A0305")
	// 两键同时为空也是「相同」：空串是书籍级评论这个合法键，不能往自己身上迁
	assertErrorCode(t, migrateKey(t, router, token, "", "").Body.Bytes(), "A0305")
}

// TestCommentHandler_MigrateKey_BookLevelToChapter 旧键为空 = 把书籍级评论归到某章节。
//
// 这是合并书籍时最常见的方向（早期把两键都标 binding:"required"，此路直接不通）。
func TestCommentHandler_MigrateKey_BookLevelToChapter(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	_, token := registerUser(t, router, "booklevel@example.com")
	createComment(t, router, token, "b1", "")
	createComment(t, router, token, "b2", "")
	createComment(t, router, token, "c1", "key-a")

	data := decodeData(t, migrateKey(t, router, token, "", "ch-1").Body.Bytes())
	if data["migrated_count"].(float64) != 2 {
		t.Errorf("Expected 2 book-level comments migrated, got %v", data["migrated_count"])
	}
	if got := commentTotalUnder(t, router, "ch-1"); got != 2 {
		t.Errorf("Expected 2 comments under ch-1, got %v", got)
	}
	if got := commentTotalUnder(t, router, "key-a"); got != 1 {
		t.Errorf("Expected unrelated chapter untouched, got %v", got)
	}
}

// TestCommentHandler_MigrateKey_ChapterToBookLevel 新键为空 = 把某章节评论汇入书籍级。
func TestCommentHandler_MigrateKey_ChapterToBookLevel(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	_, token := registerUser(t, router, "tobook@example.com")
	createComment(t, router, token, "c1", "key-a")
	createComment(t, router, token, "c2", "key-a")

	data := decodeData(t, migrateKey(t, router, token, "key-a", "").Body.Bytes())
	if data["migrated_count"].(float64) != 2 {
		t.Errorf("Expected 2 comments merged into book level, got %v", data["migrated_count"])
	}
	if got := commentTotalUnder(t, router, "key-a"); got != 0 {
		t.Errorf("Expected source chapter emptied, got %v", got)
	}
}

// TestCommentHandler_MigrateKey_OversizedKey 迁移键受写入该列的同一长度上限约束。
func TestCommentHandler_MigrateKey_OversizedKey(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	_, token := registerUser(t, router, "longkey@example.com")
	tooLong := strings.Repeat("a", maxChapterURLFilterLength+1)

	assertErrorCode(t, migrateKey(t, router, token, tooLong, "new").Body.Bytes(), "A0400")
	assertErrorCode(t, migrateKey(t, router, token, "old", tooLong).Body.Bytes(), "A0400")
}

// TestCommentHandler_MigrateKey_UserIsolation 只迁自己的评论，他人同键评论不受影响。
func TestCommentHandler_MigrateKey_UserIsolation(t *testing.T) {
	app := newTestApp(t)
	router := setupRouter()
	commentRoutes(t, router, app)

	_, token1 := registerUser(t, router, "iso_u1@example.com")
	_, token2 := registerUser(t, router, "iso_u2@example.com")
	createComment(t, router, token1, "mine", "shared-key")
	createComment(t, router, token2, "theirs", "shared-key")

	data := decodeData(t, migrateKey(t, router, token1, "shared-key", "new-key").Body.Bytes())
	if data["migrated_count"].(float64) != 1 {
		t.Errorf("Expected 1 migrated (user isolation), got %v", data["migrated_count"])
	}
	if got := commentTotalUnder(t, router, "shared-key"); got != 1 {
		t.Errorf("Expected other user's comment left under shared key, got %v", got)
	}
}

// TestChapterURLLengthLimitMatchesModelBinding 钉住读路径与写路径对同一列的长度约束。
//
// struct tag 只能写字面量、拼不了常量，所以 handler 的 maxChapterURLFilterLength 与
// model 上的 `max=2048` 是两份必须人工同步的数字——用反射比一次，漏同步当场失败。
func TestChapterURLLengthLimitMatchesModelBinding(t *testing.T) {
	cases := []struct {
		typeName string
		field    string
	}{
		{"CreateCommentRequest", "ChapterURL"},
		{"MigrateCommentKeyRequest", "OldKey"},
		{"MigrateCommentKeyRequest", "NewKey"},
	}

	for _, c := range cases {
		requestType := reflect.TypeOf(model.CreateCommentRequest{})
		if c.typeName == "MigrateCommentKeyRequest" {
			requestType = reflect.TypeOf(model.MigrateCommentKeyRequest{})
		}
		field, ok := requestType.FieldByName(c.field)
		if !ok {
			t.Fatalf("%s has no field %s", c.typeName, c.field)
		}
		maxValue := bindingMax(t, field.Tag.Get("binding"))
		if maxValue != maxChapterURLFilterLength {
			t.Errorf("%s.%s binding max=%d, want handler limit %d",
				c.typeName, c.field, maxValue, maxChapterURLFilterLength)
		}
	}
}

// bindingMax 从 binding tag（如 "omitempty,max=2048"）里取出 max 的数值。
func bindingMax(t *testing.T, tag string) int {
	t.Helper()
	for _, part := range strings.Split(tag, ",") {
		if value, found := strings.CutPrefix(part, "max="); found {
			maxValue, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("bad max in binding tag %q: %v", tag, err)
			}
			return maxValue
		}
	}
	t.Fatalf("binding tag %q has no max", tag)
	return 0
}
