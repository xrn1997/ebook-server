package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"ebook-server/model"

	"github.com/gin-gonic/gin"
)

func TestCommentHandler_GetList_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "list@example.com")

	for i := 0; i < 3; i++ {
		w := doJSON(t, router, http.MethodPost, "/api/comments", map[string]string{
			"content": "Comment " + strconv.Itoa(i), "comment_key": "ck1:list",
		}, token)
		assertErrorCode(t, w.Body.Bytes(), "00000")
	}

	w := doJSON(t, router, http.MethodGet, "/api/comments?page=1&page_size=10", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	decodeData(t, w.Body.Bytes())
}

// TestCommentHandler_Create_WithChapter 创建带章节快照的评论并校验响应视图契约
// （ADR-0011 + M2）：comment_key 原样回显、chapter_name/book_name 只作展示快照、
// user 只含 uid/username/nickname/avatar 四字段（无 email）、add_time 为格式化字符串。
func TestCommentHandler_Create_WithChapter(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "chapter_create@example.com")

	body := map[string]interface{}{
		"content":      "章节评论内容",
		"comment_key":  "ck1:aaa111#2",
		"chapter_url":  "https://src.example.com/book/1/2.html",
		"chapter_name": "第二章",
		"book_name":    "天启之书",
	}
	w := doJSON(t, router, http.MethodPost, "/api/comments", body, token)

	data := decodeData(t, w.Body.Bytes())
	if data["comment_key"] != body["comment_key"] {
		t.Errorf("comment_key not echoed: want %v, got %v", body["comment_key"], data["comment_key"])
	}
	// 展示快照与旧章节键原样存、原样回：服务端不消费书源数据，也不改写它们
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

// TestCommentHandler_GetList_ByChapter 已废弃的 chapter_url 过滤仍能读出历史行（M2 兼容路径）。
//
// 旧行**必须直接入库**：创建端点在 M2 后强制要求 comment_key，chapter_url 时代那种
// 「只带章节 URL」的请求已经发不出来了——这正是该兼容读路径存在的全部理由：
// 让换键之前就存在的数据继续可见，而不是让新客户端有条写入路径。
func TestCommentHandler_GetList_ByChapter(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	uid, token := registerUser(t, router, "chapter_list_h@example.com")
	urlA := "https://src.example.com/book/1/2.html"
	seedLegacyComment(t, app, uid, "A1", urlA)
	seedLegacyComment(t, app, uid, "A2", urlA)
	// 一条 M2 新行：有 comment_key、没有 chapter_url，不该被 chapter_url 过滤命中
	createComment(t, router, token, "新行", "ck1:brandnew")

	w := doJSON(t, router, http.MethodGet,
		"/api/comments?chapter_url="+url.QueryEscape(urlA), nil, "")
	data := decodeData(t, w.Body.Bytes())
	if data["total"].(float64) != 2 {
		t.Errorf("Expected total 2 for legacy chapter_url filter, got %v", data["total"])
	}
}

// seedLegacyComment 直接入库一条 chapter_url 时代的历史行（comment_key 为空）。
//
// 绕过 HTTP 层是刻意的：见 TestCommentHandler_GetList_ByChapter 的说明。
func seedLegacyComment(t *testing.T, app *testApp, uid uint, content, chapterURL string) {
	t.Helper()
	if err := app.db.Create(&model.Comment{
		UserID: uid, Content: content, ChapterURL: chapterURL,
	}).Error; err != nil {
		t.Fatalf("seed legacy comment failed: %v", err)
	}
}

func TestCommentHandler_GetList_Empty(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodGet, "/api/comments", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
}

func TestCommentHandler_Create_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "create@example.com")

	w := doJSON(t, router, http.MethodPost, "/api/comments",
		map[string]string{"content": "This is a test comment", "comment_key": "ck1:create"}, token)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	decodeData(t, w.Body.Bytes())
}

func TestCommentHandler_Create_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodPost, "/api/comments",
		map[string]string{"content": "This is a comment"}, "")
	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestCommentHandler_Create_EmptyContent(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "empty@example.com")

	w := doJSON(t, router, http.MethodPost, "/api/comments",
		map[string]string{"content": "", "comment_key": "ck1:empty"}, token)
	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

// TestCommentHandler_Create_MissingCommentKey 不带 comment_key 的创建必须当场被拒。
//
// 这是 M2 换轨留下的最重要一条防线：服务端若像 chapter_url 那样把这个字段当成
// 可选、静默收下缺键的请求，评论就会落进 comment_key 为空串的桶——而任何
// comment_keys 查询都不命中空串，那条评论等于写进去就丢了，且没有任何报错迹象。
// 宁可回 A0400 让客户端立刻发现，也不要这种静默丢失。
func TestCommentHandler_Create_MissingCommentKey(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "nokey@example.com")

	w := doJSON(t, router, http.MethodPost, "/api/comments",
		map[string]string{"content": "缺键的评论"}, token)
	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

// TestCommentHandler_Create_FullClientEntity 客户端原样发送整个 Comment 实体时必须正常落库。
//
// 客户端的 create 请求体不是按本端点的最小契约拼的，而是把领域里的评论对象整个序列化
// 出来（带 id、嵌套 user、add_time，以及已废弃的 chapter_url）。服务端只认自己声明的字段、
// 忽略其余，这条用例把这个「多余字段不得致错」的边界钉住——否则客户端一改实体就 400。
func TestCommentHandler_Create_FullClientEntity(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	uid, token := registerUser(t, router, "entity@example.com")

	raw := `{
	  "id": 0,
	  "user": {"uid": 0, "username": "", "nickname": "", "avatar": "", "email": ""},
	  "comment_key": "ck1:feedface#7",
	  "chapter_url": "https://src.example.com/book/9/8.html",
	  "chapter_name": "第八章",
	  "book_name": "冗余快照之书",
	  "content": "按客户端实体形态发出的评论",
	  "add_time": ""
	}`
	w := doJSON(t, router, http.MethodPost, "/api/comments", json.RawMessage(raw), token)
	data := decodeData(t, w.Body.Bytes())

	if data["comment_key"] != "ck1:feedface#7" {
		t.Errorf("comment_key not persisted: %v", data["comment_key"])
	}
	if data["content"] != "按客户端实体形态发出的评论" {
		t.Errorf("content not persisted: %v", data["content"])
	}
	// 归属由 token 决定，客户端塞进来的 user/id 一律不作数
	items := decodeData(t, doJSON(t, router, http.MethodGet,
		"/api/comments?comment_keys=ck1%3Afeedface%237", nil, "").Body.Bytes())["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("Expected the new comment findable by its key, got %d", len(items))
	}
	if owner := items[0].(map[string]interface{})["user"].(map[string]interface{})["uid"].(float64); uint(owner) != uid {
		t.Errorf("comment owner must come from the token: want uid %d, got %v", uid, owner)
	}
}

func TestCommentHandler_Delete_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "delete@example.com")

	w := doJSON(t, router, http.MethodPost, "/api/comments",
		map[string]string{"content": "Comment to delete", "comment_key": "ck1:delete"}, token)
	data := decodeData(t, w.Body.Bytes())
	commentID := uint(data["id"].(float64))

	w = doJSON(t, router, http.MethodDelete, "/api/comments/"+strconv.Itoa(int(commentID)), nil, token)
	// HTTP 恒为 200（ADR-0001），只看状态码连 C0500/A0304 都会放过
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	assertErrorCode(t, w.Body.Bytes(), "00000")
	var envelope map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode delete envelope: %v", err)
	}
	if _, exists := envelope["data"]; exists {
		t.Error("删除成功不应带 data 键（ADR-0001）")
	}
	if envelope["error"] != "" {
		t.Errorf("删除成功的 error = %v, want 空串", envelope["error"])
	}
}

func TestCommentHandler_Delete_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodDelete, "/api/comments/1", nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestCommentHandler_Delete_InvalidID(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "invalid@example.com")

	w := doJSON(t, router, http.MethodDelete, "/api/comments/invalid", nil, token)
	assertErrorCode(t, w.Body.Bytes(), "A0400")
}

func TestCommentHandler_Delete_NoPermission(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token1 := registerUser(t, router, "user1@example.com")
	_, token2 := registerUser(t, router, "user2@example.com")

	// user1 创建评论
	w := doJSON(t, router, http.MethodPost, "/api/comments",
		map[string]string{"content": "Comment by user1", "comment_key": "ck1:noperm"}, token1)
	data := decodeData(t, w.Body.Bytes())
	commentID := uint(data["id"].(float64))

	// user2 尝试删除 user1 的评论（ADR-0011：仅本人可删，专用码 A0303）
	w = doJSON(t, router, http.MethodDelete, "/api/comments/"+strconv.Itoa(int(commentID)), nil, token2)
	assertErrorCode(t, w.Body.Bytes(), "A0303")
}

func TestCommentHandler_GetMyComments_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "mycomments@example.com")

	doJSON(t, router, http.MethodPost, "/api/comments",
		map[string]string{"content": "My comment", "comment_key": "ck1:mine"}, token)

	w := doJSON(t, router, http.MethodGet, "/api/comments/my", nil, token)
	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 1 {
		t.Errorf("Expected 1 comment, got %d", len(items))
	}
}

func TestCommentHandler_GetMyComments_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodGet, "/api/comments/my", nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestCommentHandler_GetList_Pagination(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "paginate@example.com")

	for i := 0; i < 15; i++ {
		doJSON(t, router, http.MethodPost, "/api/comments",
			map[string]string{"content": "Comment " + strconv.Itoa(i), "comment_key": "ck1:paging"}, token)
	}

	w := doJSON(t, router, http.MethodGet, "/api/comments?page=1&page_size=5", nil, "")
	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 5 {
		t.Errorf("Expected 5 items on page 1, got %d", len(items))
	}
	if data["page"].(float64) != 1 {
		t.Errorf("Expected page 1, got %v", data["page"])
	}

	w = doJSON(t, router, http.MethodGet, "/api/comments?page=2&page_size=5", nil, "")
	data = decodeData(t, w.Body.Bytes())
	items = data["items"].([]interface{})
	if len(items) != 5 {
		t.Errorf("Expected 5 items on page 2, got %d", len(items))
	}
}

func TestCommentHandler_GetMyComments_Pagination(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "mypaginate@example.com")

	for i := 0; i < 5; i++ {
		doJSON(t, router, http.MethodPost, "/api/comments",
			map[string]string{"content": "Comment " + strconv.Itoa(i), "comment_key": "ck1:paging"}, token)
	}

	w := doJSON(t, router, http.MethodGet, "/api/comments/my?page=1&page_size=3", nil, token)
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
	router := app.engine

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
	router := app.engine

	_, token := registerUser(t, router, "notfound@example.com")

	w := doJSON(t, router, http.MethodDelete, "/api/comments/999999", nil, token)
	// 评论不存在 → 评论域专用码 A0304（ADR-0011）
	assertErrorCode(t, w.Body.Bytes(), "A0304")
}

// TestCommentHandler_GetList_MultiCommentKeys 逗号分隔的多个 comment_keys 返回并集。
//
// 这正是真实客户端的形态：Retrofit 用单个 @Query 把跨书源合并的多个键拼成一个逗号串，
// 所以这里必须验「一个参数里的多个键」，而不是重复参数。
func TestCommentHandler_GetList_MultiCommentKeys(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "multi_keys@example.com")
	createComment(t, router, token, "A1", "ck1:a#0")
	createComment(t, router, token, "A2", "ck1:a#0")
	createComment(t, router, token, "B1", "ck1:b#1")
	createComment(t, router, token, "C1", "ck1:c#2")

	// 并集 = ck1:a#0 两条 + ck1:b#1 一条；ck1:c#2 不在查询内
	w := doJSON(t, router, http.MethodGet,
		"/api/comments?comment_keys=ck1%3Aa%230,ck1%3Ab%231", nil, "")
	data := decodeData(t, w.Body.Bytes())
	if data["total"].(float64) != 3 {
		t.Errorf("Expected union total 3, got %v", data["total"])
	}
}

// TestCommentHandler_GetList_SingleCommentKey 单键即精确匹配该桶（无等值分支）。
//
// 章评键带 # 后缀、作品键不带，服务端一视同仁当不透明字符串——刻意两种都验，
// 因为「不解析 #」正是 M2 把键派生交给客户端的前提。
func TestCommentHandler_GetList_SingleCommentKey(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "single_key@example.com")
	createComment(t, router, token, "A1", "ck1:a")
	createComment(t, router, token, "A2", "ck1:a")
	createComment(t, router, token, "B1", "ck1:b")
	createComment(t, router, token, "章评", "ck1:a#3")

	if got := commentTotalUnder(t, router, "ck1:a"); got != 2 {
		t.Errorf("Expected total 2 for single key, got %v", got)
	}
	if got := commentTotalUnder(t, router, "ck1:a#3"); got != 1 {
		t.Errorf("Expected chapter key matched exactly, not prefix-matched, got %v", got)
	}
}

// TestCommentHandler_GetList_CommentKeysSkipsLegacyRows 未换键的历史行不被任何 comment_keys 命中。
//
// 这些行的 comment_key 是空串，只能靠已废弃的 chapter_url 参数读出来——
// 空串不参与 IN 匹配，否则随便一个查询都会把全站旧行捞进来。
func TestCommentHandler_GetList_CommentKeysSkipsLegacyRows(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	uid, _ := registerUser(t, router, "legacy_skip@example.com")
	seedLegacyComment(t, app, uid, "旧行1", "https://src.example.com/1.html")
	seedLegacyComment(t, app, uid, "旧行2", "https://src.example.com/2.html")

	w := doJSON(t, router, http.MethodGet, "/api/comments?comment_keys=ck1:a", nil, "")
	data := decodeData(t, w.Body.Bytes())
	if data["total"].(float64) != 0 {
		t.Errorf("Expected legacy rows not matched by comment_keys, got %v", data["total"])
	}
}

// TestCommentHandler_GetList_CommentKeysPrecedence comment_keys 提供时旧参数不参与过滤。
//
// 两个键参数同时出现属于客户端混用新旧契约，必须有一方明确胜出，否则结果是交集
// 还是并集全靠实现顺序——这里定成 M2 主路径优先，书名也不再收窄（它已不是聚合维度）。
func TestCommentHandler_GetList_CommentKeysPrecedence(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	uid, token := registerUser(t, router, "precedence@example.com")
	createComment(t, router, token, "新键行", "ck1:win")
	// 旧键行：只有 chapter_url、没有 comment_key。若旧参数参与过滤就会被一起捞出来
	seedLegacyComment(t, app, uid, "旧键行", "legacy-url")

	w := doJSON(t, router, http.MethodGet,
		"/api/comments?comment_keys=ck1%3Awin&chapter_url=legacy-url", nil, "")
	data := decodeData(t, w.Body.Bytes())
	if data["total"].(float64) != 1 {
		t.Errorf("Expected comment_keys to win (total 1), got %v", data["total"])
	}
}

// TestCommentHandler_GetList_EmptyCommentKeysIgnored ?comment_keys= 不带值按未提供处理。
//
// 空串在 M2 里虽然对应「未换键的历史行」这个桶，但查询串里留空通常是客户端
// 没有可用键（例如键列表为空时直接不透传），因此退化为全局最新列表而不是空集。
func TestCommentHandler_GetList_EmptyCommentKeysIgnored(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "empty_keys@example.com")
	createComment(t, router, token, "带键的行", "ck1:some")

	w := doJSON(t, router, http.MethodGet, "/api/comments?comment_keys=", nil, "")
	data := decodeData(t, w.Body.Bytes())
	if data["total"].(float64) != 1 {
		t.Errorf("Expected empty filter to be ignored (total 1), got %v", data["total"])
	}
}

// TestCommentHandler_GetList_EmptyChapterURLLegacyIgnored ?chapter_url= 不带值按未提供处理。
//
// chapter_url 已降级为兼容读路径，但空串仍是不可查询的键（Normalize 会把空键判成
// A0400）；查询串里留空只是没填筛选条件，应退化为全站最新列表而不是报错。
func TestCommentHandler_GetList_EmptyChapterURLLegacyIgnored(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "empty_legacy_url@example.com")
	createComment(t, router, token, "有键的行", "ck1:legacy")

	w := doJSON(t, router, http.MethodGet, "/api/comments?chapter_url=", nil, "")
	data := decodeData(t, w.Body.Bytes())
	if data["total"].(float64) != 1 {
		t.Errorf("Expected empty chapter_url to be ignored (total 1), got %v", data["total"])
	}
}

// TestCommentHandler_GetList_CommentKeysLimits comment_keys 的两条入参上限都必须生效。
//
// 数量上限防的是「把 SQLite 单语句绑定变量打满」，长度上限防的是永远匹配不到的无效键；
// 越界与恰好在上限内两种情形都验，否则上限写错方向（off-by-one）不会被发现。
func TestCommentHandler_GetList_CommentKeysLimits(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	// 超出数量上限
	w := doJSON(t, router, http.MethodGet,
		"/api/comments?comment_keys="+keysQuery(model.MaxCommentFilterKeys+1), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0400")
	// 超出单键长度上限
	w = doJSON(t, router, http.MethodGet,
		"/api/comments?comment_keys="+strings.Repeat("a", model.MaxCommentKeyLength+1), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0400")
	// 恰好在上限内不得误伤
	w = doJSON(t, router, http.MethodGet,
		"/api/comments?comment_keys="+keysQuery(model.MaxCommentFilterKeys), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "00000")
	w = doJSON(t, router, http.MethodGet,
		"/api/comments?comment_keys="+strings.Repeat("a", model.MaxCommentKeyLength), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "00000")
}

// keysQuery 拼出 n 个逗号分隔的 comment_keys（键名唯一，便于以后按内容断言）。
func keysQuery(n int) string {
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf("k%d", i))
	}
	return strings.Join(parts, ",")
}

// TestCommentHandler_GetList_ChapterURLLimits 已废弃的 chapter_url 路径两条上限仍然生效。
//
// 兼容路径不是「放着不管」的代码：它同样面向未认证流量，两条上限必须与换键前一致。
func TestCommentHandler_GetList_ChapterURLLimits(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodGet, "/api/comments?"+chapterURLQuery(model.MaxCommentFilterKeys+1), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0400")
	w = doJSON(t, router, http.MethodGet, "/api/comments?chapter_url="+strings.Repeat("a", model.MaxChapterURLLength+1), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0400")
	w = doJSON(t, router, http.MethodGet, "/api/comments?"+chapterURLQuery(model.MaxCommentFilterKeys), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "00000")
	w = doJSON(t, router, http.MethodGet, "/api/comments?chapter_url="+strings.Repeat("a", model.MaxChapterURLLength), nil, "")
	assertErrorCode(t, w.Body.Bytes(), "00000")
}

// chapterURLQuery 拼出 n 个 chapter_url 参数的查询串（重复参数形态，旧客户端的用法）。
func chapterURLQuery(n int) string {
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf("chapter_url=k%d", i))
	}
	return strings.Join(parts, "&")
}

// commentTotalUnder 查某 comment_key 聚合键下的评论总数（走公开列表端点，匿名即可）。
func commentTotalUnder(t *testing.T, router *gin.Engine, key string) float64 {
	t.Helper()
	w := doJSON(t, router, http.MethodGet,
		"/api/comments?comment_keys="+url.QueryEscape(key), nil, "")
	data := decodeData(t, w.Body.Bytes())
	return data["total"].(float64)
}

// migrateKey 以 token 身份发一次聚合键迁移请求（token 传空串 = 匿名）。
//
// 路径是 M2 契约规定的 /api/comments/migrate（旧名 migrate-key 已随换轨一并改掉）。
func migrateKey(t *testing.T, router *gin.Engine, token, oldKey, newKey string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, router, http.MethodPost, "/api/comments/migrate", map[string]string{
		"old_key": oldKey,
		"new_key": newKey,
	}, token)
}

// TestCommentHandler_MigrateKey_Success 迁移后评论只在新键下可见，且重复迁移为幂等。
func TestCommentHandler_MigrateKey_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

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
	router := app.engine

	assertErrorCode(t, migrateKey(t, router, "", "a", "b").Body.Bytes(), "A0230")
}

// TestCommentHandler_MigrateKey_SameKey 新旧相同返回 A0305，而不是静默「迁移 0 行」。
func TestCommentHandler_MigrateKey_SameKey(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "samekey@example.com")

	assertErrorCode(t, migrateKey(t, router, token, "same", "same").Body.Bytes(), "A0305")
	// 两键同时为空改判 A0400：空 new_key 在 required 上就被拒，走不到「新旧相同」那一步
	assertErrorCode(t, migrateKey(t, router, token, "", "").Body.Bytes(), "A0400")
}

// TestCommentHandler_MigrateKey_LegacyUnkeyedRows 旧键为空 = 把本人未换键的历史行收进正确的桶。
//
// 这是换轨后最实用的一次性动作：M2 之前入库的行 comment_key 全为空串，用户改了书名作者
// 算出正确的 ck1 键后，可以把自己的旧评论直接迁进去。
// 旧行用 seedLegacyComment 造——创建端点现在强制要求带键，空键请求会被当场拒绝。
func TestCommentHandler_MigrateKey_LegacyUnkeyedRows(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	uid, token := registerUser(t, router, "unkeyed@example.com")
	seedLegacyComment(t, app, uid, "b1", "https://src.example.com/1.html")
	seedLegacyComment(t, app, uid, "b2", "https://src.example.com/2.html")
	createComment(t, router, token, "c1", "key-a")

	data := decodeData(t, migrateKey(t, router, token, "", "ch-1").Body.Bytes())
	if data["migrated_count"].(float64) != 2 {
		t.Errorf("Expected 2 unkeyed comments migrated, got %v", data["migrated_count"])
	}
	if got := commentTotalUnder(t, router, "ch-1"); got != 2 {
		t.Errorf("Expected 2 comments under ch-1, got %v", got)
	}
	if got := commentTotalUnder(t, router, "key-a"); got != 1 {
		t.Errorf("Expected unrelated key untouched, got %v", got)
	}
}

// TestCommentHandler_MigrateKey_RejectsEmptyNewKey 迁往空新键被拒（A0400）。
//
// 空 new_key 会把评论推进一个没有任何 comment_keys 查询能命中的黑洞——与「创建评论必须
// 带键」致命的正是同一件事，因此不能从迁移侧开后门。旧键方向仍允许空（收拢历史行）。
func TestCommentHandler_MigrateKey_RejectsEmptyNewKey(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "emptynew@example.com")
	createComment(t, router, token, "c1", "key-a")

	assertErrorCode(t, migrateKey(t, router, token, "key-a", "").Body.Bytes(), "A0400")
	// 被拒后原桶不受影响：不能出现「校验失败但数据已动」
	if got := commentTotalUnder(t, router, "key-a"); got != 1 {
		t.Errorf("Expected source bucket untouched after rejection, got %v", got)
	}
}

// TestCommentHandler_MigrateKey_OversizedKey 迁移键受写入 comment_key 列的同一长度上限约束。
func TestCommentHandler_MigrateKey_OversizedKey(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	_, token := registerUser(t, router, "longkey@example.com")
	tooLong := strings.Repeat("a", model.MaxCommentKeyLength+1)

	assertErrorCode(t, migrateKey(t, router, token, tooLong, "new").Body.Bytes(), "A0400")
	assertErrorCode(t, migrateKey(t, router, token, "old", tooLong).Body.Bytes(), "A0400")
}

// TestCommentHandler_MigrateKey_UserIsolation 只迁自己的评论，他人同键评论不受影响。
func TestCommentHandler_MigrateKey_UserIsolation(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

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
