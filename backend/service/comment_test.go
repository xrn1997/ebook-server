package service

import (
	"errors"
	"strings"
	"testing"

	"ebook-server/model"
)

func TestCommentService_Create_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 先创建用户
	authService := testAuth
	user := serviceRegister(t, authService, "commentuser@example.com", "password123")

	// 创建评论
	commentService := testComments
	createReq := &model.CreateCommentRequest{
		Content:    "This is a test comment",
		CommentKey: "ck1:create",
	}

	comment, err := commentService.Create(testCtx, user.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// Create 返回响应视图（ADR-0011）：content 透传，作者经 user.uid 呈现
	if comment.Content != createReq.Content {
		t.Errorf("Expected content '%s', got '%s'", createReq.Content, comment.Content)
	}

	if comment.User.UID != user.UID {
		t.Errorf("Expected User UID %d, got %d", user.UID, comment.User.UID)
	}
}

// TestCommentService_Create_RejectsEmptyKey 缺键必须由模块本身挡住。
//
// handler 的 binding=required 只覆盖 HTTP 路径；空键写进 comments 就是一条任何
// comment_keys 查询都命中不到、也没有迁移方向能读回来的行（ADR-0014）。
func TestCommentService_Create_RejectsEmptyKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "empty-key@example.com", "password123")

	_, err := testComments.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "缺键"})
	if !errors.Is(err, model.ErrCommentKeyInvalid) {
		t.Fatalf("err = %v, want ErrCommentKeyInvalid", err)
	}

	total, err := testComments.Count(testCtx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if total != 0 {
		t.Fatalf("缺键评论不应落库, total = %d", total)
	}
}

// TestCommentService_Create_RejectsOversizedMultibyteKey binding 的 max=200 按字符
// 计数，列约束按字节描述；多字节键必须在模块边界再判一次，否则「建得进去、迁不出去」。
func TestCommentService_Create_RejectsOversizedMultibyteKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "multibyte-key@example.com", "password123")
	key := strings.Repeat("评", model.MaxCommentKeyLength/3+1) // 201 字节

	_, err := testComments.Create(testCtx, user.UID, &model.CreateCommentRequest{
		Content:    "x",
		CommentKey: key,
	})
	if !errors.Is(err, model.ErrCommentKeyInvalid) {
		t.Fatalf("err = %v, want ErrCommentKeyInvalid", err)
	}
	if !strings.Contains(err.Error(), "最长") {
		t.Errorf("error should explain the byte limit: %v", err)
	}
}

// TestCommentService_Create_RejectsOversizedMultibyteChapterURL chapter_url 的读路径
// 按字节限长；创建若只靠 binding 的字符上限，多字节 URL 会写进一条永远查不回来的行。
func TestCommentService_Create_RejectsOversizedMultibyteChapterURL(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "multibyte-url@example.com", "password123")
	url := strings.Repeat("章", model.MaxChapterURLLength) // 6144 字节，2048 个字符

	_, err := testComments.Create(testCtx, user.UID, &model.CreateCommentRequest{
		Content:    "x",
		CommentKey: "ck1:legacy-url",
		ChapterURL: url,
	})
	if !errors.Is(err, model.ErrCommentKeyInvalid) {
		t.Fatalf("err = %v, want ErrCommentKeyInvalid", err)
	}
	if !strings.Contains(err.Error(), "章节地址最长") {
		t.Errorf("error should explain the chapter_url byte limit: %v", err)
	}
}

// TestCommentService_Create_WithChapter 创建带聚合键与章节归属的评论（M2 + ADR-0011）。
//
// comment_key 是唯一的聚合键（必填），chapter_url/chapter_name/book_name 只是
// 过渡期兼容与展示快照——两列同时写入，旧客户端与新客户端才都能读到。
func TestCommentService_Create_WithChapter(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "chapter_user@example.com", "password123")

	commentService := testComments
	createReq := &model.CreateCommentRequest{
		Content:     "章节评论",
		CommentKey:  "ck1:9f2c#2",
		ChapterURL:  "https://src.example.com/book/1/2.html",
		ChapterName: "第二章",
		BookName:    "天启之书",
	}
	comment, err := commentService.Create(testCtx, user.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create chapter comment: %v", err)
	}

	if comment.CommentKey != createReq.CommentKey {
		t.Errorf("comment_key not persisted: got %q, want %q", comment.CommentKey, createReq.CommentKey)
	}
	if comment.ChapterURL != createReq.ChapterURL ||
		comment.ChapterName != createReq.ChapterName ||
		comment.BookName != createReq.BookName {
		t.Errorf("chapter fields not persisted: %+v", comment)
	}
}

// ── M2 主读路径：List(CommentKeys) ────────────────────────────────────────

// TestCommentService_List_CommentKeys_SingleKey 单键查询即精确匹配该键。
//
// 键由客户端派生、服务端不解释，所以断言只看命中集合与作者视图，不校验键形态。
func TestCommentService_List_CommentKeys_SingleKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "ck_single@example.com", "password123")
	commentService := testComments

	keyA := "ck1:aaaa#1"
	keyB := "ck1:aaaa#2"
	for i := 0; i < 2; i++ {
		commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "A", CommentKey: keyA, BookName: "书A"})
	}
	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "B", CommentKey: keyB, BookName: "书A"})

	result, err := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{keyA}, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(CommentKeys) failed: %v", err)
	}
	if result.Total != 2 || len(result.Items) != 2 {
		t.Errorf("single key = total %d len %d, want 2/2", result.Total, len(result.Items))
	}
	for _, item := range result.Items {
		if item.CommentKey != keyA {
			t.Errorf("item comment_key = %q, want %q", item.CommentKey, keyA)
		}
		// 作者视图必须填充：评论响应只暴露 uid/username/nickname/avatar
		if item.User.UID != user.UID {
			t.Errorf("item author UID = %d, want %d", item.User.UID, user.UID)
		}
	}
}

// TestCommentService_List_CommentKeys_Union 多键返回并集（跨书源合并同一作品）。
func TestCommentService_List_CommentKeys_Union(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "ck_union@example.com", "password123")
	commentService := testComments

	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "A1", CommentKey: "ck1:a#1"})
	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "A2", CommentKey: "ck1:a#1"})
	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "B1", CommentKey: "ck1:b#1"})
	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "C1", CommentKey: "ck1:c#1"})

	result, err := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{"ck1:a#1", "ck1:b#1"}, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(CommentKeys) union failed: %v", err)
	}
	if result.Total != 3 || len(result.Items) != 3 {
		t.Errorf("union = total %d len %d, want 3/3", result.Total, len(result.Items))
	}
}

// TestCommentService_List_CommentKeys_Pagination 并集分页并把分页参数归一化。
func TestCommentService_List_CommentKeys_Pagination(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "ck_pg@example.com", "password123")
	commentService := testComments

	for i := 0; i < 5; i++ {
		commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "A", CommentKey: "ck1:a"})
	}
	for i := 0; i < 3; i++ {
		commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "B", CommentKey: "ck1:b"})
	}

	keys := []string{"ck1:a", "ck1:b"}
	page1, err := commentService.List(testCtx, model.CommentFilter{CommentKeys: keys, Page: 1, PageSize: 3})
	if err != nil {
		t.Fatalf("List(CommentKeys) page 1 failed: %v", err)
	}
	if page1.Total != 8 || len(page1.Items) != 3 {
		t.Errorf("page 1 = total %d len %d, want 8/3", page1.Total, len(page1.Items))
	}
	if page1.Page != 1 || page1.PageSize != 3 {
		t.Errorf("page 1 echo = (%d,%d), want (1,3)", page1.Page, page1.PageSize)
	}

	// 末页只剩 2 条
	page3, _ := commentService.List(testCtx, model.CommentFilter{CommentKeys: keys, Page: 3, PageSize: 3})
	if len(page3.Items) != 2 || page3.Total != 8 {
		t.Errorf("page 3 = len %d total %d, want 2/8", len(page3.Items), page3.Total)
	}

	// page/page_size 非法值归一化为 1/10（全站列表接口的统一约定）
	normalized, _ := commentService.List(testCtx, model.CommentFilter{CommentKeys: keys, Page: 0, PageSize: 0})
	if normalized.Page != 1 || normalized.PageSize != 10 {
		t.Errorf("normalized = (%d,%d), want (1,10)", normalized.Page, normalized.PageSize)
	}
}

// TestCommentService_List_CommentKeys_SkipsUnkeyedRows 空键不属于任何桶。
//
// 空 comment_key = 尚未换键的历史行。它们必须从非空键过滤里消失，否则客户端一旦
// 拼错键，就会把无主评论当成自己章节的内容显示出来。
func TestCommentService_List_CommentKeys_SkipsUnkeyedRows(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "ck_unkeyed@example.com", "password123")
	commentService := testComments

	seedLegacyComment(t, user.UID, "无键旧评", "", "")
	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "有键", CommentKey: "ck1:a#1"})

	result, err := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{"ck1:a#1"}, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(CommentKeys) failed: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Content != "有键" {
		t.Errorf("filter result = %+v, want only 有键", result.Items)
	}
	if result.Items[0].CommentKey != "ck1:a#1" {
		t.Errorf("item comment_key = %q, want ck1:a#1", result.Items[0].CommentKey)
	}
}

// TestCommentService_LegacyChapterURLStillReadable 未换键的历史行仍经旧路径可读。
//
// 服务端无法把 chapter_url 重算成 comment_key（算键需要作者，而作者从未入库），
// 所以废弃读路径是这些评论唯一的出口——换聚合键不能把它们变成孤儿。
func TestCommentService_LegacyChapterURLStillReadable(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "ck_legacy@example.com", "password123")
	commentService := testComments

	legacyURL := "https://src.example.com/book/9/1.html"
	seedLegacyComment(t, user.UID, "旧行", legacyURL, "")

	legacy, err := commentService.List(testCtx, model.CommentFilter{LegacyKeys: []string{legacyURL}, BookName: "", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(LegacyKeys) failed: %v", err)
	}
	if legacy.Total != 1 || legacy.Items[0].CommentKey != "" {
		t.Errorf("legacy row = %+v, want 1 条且 comment_key 为空", legacy.Items)
	}

	// 同一个字符串不是新键：按 comment_key 查必须为空
	viaKey, err := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{legacyURL}, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(CommentKeys) failed: %v", err)
	}
	if viaKey.Total != 0 {
		t.Errorf("chapter_url value must not match comment_key, got %d", viaKey.Total)
	}
}

// TestCommentService_List_LegacyKeys_SingleKey 已废弃路径：单键即精确匹配（ADR-0011）。
//
// M2 后聚合键换成 comment_key，这里守的是「旧客户端带 chapter_url + book_name 仍能查到」
// 这条兼容承诺，不是主读路径（主路径见 List(CommentKeys)）。
func TestCommentService_List_LegacyKeys_SingleKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "chapter_list@example.com", "password123")
	commentService := testComments

	urlA := "https://src.example.com/book/1/2.html"
	urlB := "https://src.example.com/book/1/3.html"
	// 章节 A 两条、章节 B 一条、未换键（chapter_url 为空）一条
	for i := 0; i < 2; i++ {
		seedLegacyComment(t, user.UID, "A", urlA, "书A")
	}
	seedLegacyComment(t, user.UID, "B", urlB, "书B")
	seedLegacyComment(t, user.UID, "通用", "", "")

	result, err := commentService.List(testCtx, model.CommentFilter{LegacyKeys: []string{urlA}, BookName: "", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("Failed to get chapter comments: %v", err)
	}
	if result.Total != 2 || len(result.Items) != 2 {
		t.Errorf("Expected 2 comments for chapter A, got total=%d len=%d", result.Total, len(result.Items))
	}

	// book_name 二次过滤：传错书名应过滤干净
	result, err = commentService.List(testCtx, model.CommentFilter{LegacyKeys: []string{urlA}, BookName: "书B", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("Failed to get filtered comments: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Expected 0 comments for wrong book filter, got %d", result.Total)
	}
}

// TestCommentService_List_ByBookName 按书名单独过滤（ADR-0011：book_name 不依赖 chapter_url）。
func TestCommentService_List_ByBookName(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "book_filter@example.com", "password123")
	commentService := testComments

	// 书A 两个不同章节 + 书B 一条 + 无章节一条（都是 chapter_url 时代的历史行）
	seedLegacyComment(t, user.UID, "A1", "https://s/book/1/1.html", "书A")
	seedLegacyComment(t, user.UID, "A2", "https://s/book/1/2.html", "书A")
	seedLegacyComment(t, user.UID, "B", "https://s/book/2/1.html", "书B")
	seedLegacyComment(t, user.UID, "无章节", "", "")

	result, err := commentService.List(testCtx, model.CommentFilter{BookName: "书A", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("Failed to get book comments: %v", err)
	}
	if result.Total != 2 || len(result.Items) != 2 {
		t.Errorf("Expected 2 comments for 书A, got total=%d len=%d", result.Total, len(result.Items))
	}
}

func TestCommentService_List_NoFilter_Pagination(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := testComments

	// 创建一些评论
	authService := testAuth
	user := serviceRegister(t, authService, "pagination_user@example.com", "password123")
	uid := user.UID

	for i := 0; i < 15; i++ {
		commentService.Create(testCtx, uid, &model.CreateCommentRequest{
			Content:    "Comment content",
			CommentKey: "ck1:page",
		})
	}

	// 获取第一页
	result1, err := commentService.List(testCtx, model.CommentFilter{Page: 1, PageSize: 5})
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}

	if result1.Page != 1 {
		t.Errorf("Expected Page 1, got %d", result1.Page)
	}

	if result1.PageSize != 5 {
		t.Errorf("Expected PageSize 5, got %d", result1.PageSize)
	}

	if len(result1.Items) != 5 {
		t.Errorf("Expected 5 items, got %d", len(result1.Items))
	}

	// 获取第二页
	result2, err := commentService.List(testCtx, model.CommentFilter{Page: 2, PageSize: 5})
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}

	if result2.Page != 2 {
		t.Errorf("Expected Page 2, got %d", result2.Page)
	}
}

func TestCommentService_Delete_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 创建用户和评论
	authService := testAuth
	user := serviceRegister(t, authService, "deletecommentuser@example.com", "password123")

	commentService := testComments
	createReq := &model.CreateCommentRequest{
		Content:    "Comment to delete",
		CommentKey: "ck1:delete",
	}
	comment, err := commentService.Create(testCtx, user.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// 删除评论
	err = commentService.Delete(testCtx, comment.ID, model.CommentScopeOfAuthor(user.UID))
	if err != nil {
		t.Fatalf("Failed to delete comment: %v", err)
	}

	// 验证删除：软删后全局列表不再包含该评论
	list, err := commentService.List(testCtx, model.CommentFilter{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("Failed to list comments: %v", err)
	}
	for _, item := range list.Items {
		if item.ID == comment.ID {
			t.Error("deleted comment should not appear in list")
		}
	}
}

func TestCommentService_Delete_NoPermission(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 创建两个用户
	authService := testAuth
	user1 := serviceRegister(t, authService, "user1_delete@example.com", "password123")
	user2 := serviceRegister(t, authService, "user2_delete@example.com", "password123")

	// user1 创建评论
	commentService := testComments
	createReq := &model.CreateCommentRequest{
		Content:    "Comment by user1",
		CommentKey: "ck1:owner",
	}
	comment, err := commentService.Create(testCtx, user1.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// user2 尝试删除 user1 的评论
	err = commentService.Delete(testCtx, comment.ID, model.CommentScopeOfAuthor(user2.UID))
	if err != model.ErrCommentNotOwner {
		t.Errorf("Expected ErrCommentNotOwner, got %v", err)
	}
}

func TestCommentService_Delete_CommentNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := testComments

	err := commentService.Delete(testCtx, 999999, model.CommentScopeOfAuthor(1))
	if err != model.ErrCommentNotFound {
		t.Errorf("Expected ErrCommentNotFound, got %v", err)
	}
}

// TestCommentService_Delete_AdminScopeBypassesOwnership 治理删只校验存在，不校验归属；
// 第二次删除必须报 A0304，而不是「删了 0 行也算成功」。
func TestCommentService_Delete_AdminScopeBypassesOwnership(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	author := serviceRegister(t, testAuth, "cp-admin-author@example.com", "password123")
	c, err := testComments.Create(testCtx, author.UID, &model.CreateCommentRequest{Content: "governance", CommentKey: "ck1:gov"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := testComments.Delete(testCtx, c.ID, model.CommentScopeOfAdmin()); err != nil {
		t.Fatalf("admin delete: %v", err)
	}
	if err := testComments.Delete(testCtx, c.ID, model.CommentScopeOfAdmin()); err != model.ErrCommentNotFound {
		t.Fatalf("second delete = %v, want ErrCommentNotFound", err)
	}
}

// TestCommentService_ListForAdmin_FixedView 后台列表使用固定视图：含 content 与 created_at，
// user 只含展示字段。
func TestCommentService_ListForAdmin_FixedView(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	u := serviceRegister(t, testAuth, "cp-view@example.com", "password123")
	if _, err := testComments.Create(testCtx, u.UID, &model.CreateCommentRequest{
		Content: "精彩", CommentKey: "ck1:view", BookName: "书A",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	resp, err := testComments.ListForAdmin(testCtx, model.CommentQuery{Keyword: "精彩"}, 1, 10)
	if err != nil {
		t.Fatalf("ListForAdmin: %v", err)
	}
	if resp.Total != 1 || len(resp.List) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.List[0].Content != "精彩" {
		t.Errorf("content missing: %+v", resp.List[0])
	}
	if resp.List[0].User.Username != u.Username {
		t.Errorf("user view missing username: %+v", resp.List[0].User)
	}
	if resp.List[0].CreatedAt.IsZero() {
		t.Error("created_at missing")
	}
}

// TestCommentService_Count 后台概览与用户详情共用评论模块的统计入口。
func TestCommentService_Count(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	u := serviceRegister(t, testAuth, "cp-count@example.com", "password123")
	if _, err := testComments.Create(testCtx, u.UID, &model.CreateCommentRequest{Content: "c1", CommentKey: "ck1:c1"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := testComments.Create(testCtx, u.UID, &model.CreateCommentRequest{Content: "c2", CommentKey: "ck1:c2"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	total, err := testComments.Count(testCtx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if total != 2 {
		t.Errorf("Count = %d, want 2", total)
	}
	perUser, err := testComments.CountByUserID(testCtx, u.UID)
	if err != nil {
		t.Fatalf("CountByUserID: %v", err)
	}
	if perUser != 2 {
		t.Errorf("CountByUserID = %d, want 2", perUser)
	}
}

func TestCommentService_List_ByUserID_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "getbyuserid@example.com", "password123")

	commentService := testComments
	for i := 0; i < 5; i++ {
		commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "Comment", CommentKey: "ck1:mine"})
	}

	// 创建另一个用户的评论
	other := serviceRegister(t, authService, "other@example.com", "password123")
	commentService.Create(testCtx, other.UID, &model.CreateCommentRequest{Content: "Other comment", CommentKey: "ck1:other"})

	// ByUserID 必须显式：光给 UserID 不算「按本人过滤」（见 model.CommentFilter），
	// 否则 uid 为 0 的一次调用会从「查不到」变成「查所有人的评论」。
	result, err := commentService.List(testCtx, model.CommentFilter{UserID: user.UID, ByUserID: true, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if len(result.Items) != 5 {
		t.Errorf("Expected 5 comments, got %d", len(result.Items))
	}
	if result.Total != 5 {
		t.Errorf("Expected total 5, got %d", result.Total)
	}
}

func TestCommentService_List_ByUserID_Empty(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := testComments

	// 库里先放一条别人的评论：空库时「按用户过滤坏掉、退回全站列表」与「本人确实没评论」
	// 都返回 0 条，这条用例就永远不会红。
	other := serviceRegister(t, testAuth, "other-empty-list@example.com", "password123")
	commentService.Create(testCtx, other.UID, &model.CreateCommentRequest{Content: "别人的评论", CommentKey: "ck1:other-empty"})

	// ByUserID 必须显式（见 model.CommentFilter）：漏掉它就会走到全局兜底分支，
	// 断言的不再是「本人没有评论」。
	result, err := commentService.List(testCtx, model.CommentFilter{UserID: 999, ByUserID: true, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("Expected 0 comments, got %d", len(result.Items))
	}
	if result.Total != 0 {
		t.Errorf("Expected total 0, got %d", result.Total)
	}
}

func TestCommentService_List_PagingClamp(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "bounds@example.com", "password123")
	commentService := testComments

	for i := 0; i < 3; i++ {
		commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "Comment", CommentKey: "ck1:clamp"})
	}

	// page=0 应修正为 1
	result, err := commentService.List(testCtx, model.CommentFilter{Page: 0, PageSize: 10})
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if result.Page != 1 {
		t.Errorf("Expected Page 1 for page=0, got %d", result.Page)
	}

	// page=-1 应修正为 1
	result, err = commentService.List(testCtx, model.CommentFilter{Page: -1, PageSize: 10})
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if result.Page != 1 {
		t.Errorf("Expected Page 1 for page=-1, got %d", result.Page)
	}

	// pageSize=0 应修正为 10
	result, err = commentService.List(testCtx, model.CommentFilter{Page: 1, PageSize: 0})
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=0, got %d", result.PageSize)
	}

	// pageSize=101 应修正为 10
	result, err = commentService.List(testCtx, model.CommentFilter{Page: 1, PageSize: 101})
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=101, got %d", result.PageSize)
	}
}

func TestCommentService_List_LegacyKeys_Union(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "multi_url@example.com", "password123")
	commentService := testComments

	seedLegacyComment(t, user.UID, "A1", "key-a", "书A")
	seedLegacyComment(t, user.UID, "A2", "key-a", "书A")
	seedLegacyComment(t, user.UID, "B1", "key-b", "书A")
	seedLegacyComment(t, user.UID, "C1", "key-c", "书B")

	result, err := commentService.List(testCtx, model.CommentFilter{LegacyKeys: []string{"key-a", "key-b"}, BookName: "", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List(LegacyKeys) failed: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("Expected total 3, got %d", result.Total)
	}
}

// TestCommentService_MigrateKey_Success 迁移作用于 comment_key，幂等重试返回 0。
func TestCommentService_MigrateKey_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "migrate@example.com", "password123")
	commentService := testComments

	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "c1", CommentKey: "old-key"})
	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "c2", CommentKey: "old-key"})
	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "c3", CommentKey: "other-key"})

	count, err := commentService.MigrateKey(testCtx, user.UID, "old-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 migrated, got %d", count)
	}

	// 新键下应有 2 条
	result, _ := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{"new-key"}, Page: 1, PageSize: 10})
	if result.Total != 2 {
		t.Errorf("Expected 2 under new-key, got %d", result.Total)
	}
	// 旧键清空，邻键不受影响
	if old, _ := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{"old-key"}, Page: 1, PageSize: 10}); old.Total != 0 {
		t.Errorf("old-key should be empty, got %d", old.Total)
	}
	if other, _ := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{"other-key"}, Page: 1, PageSize: 10}); other.Total != 1 {
		t.Errorf("other-key should be untouched, got %d", other.Total)
	}
	// 幂等：客户端重试不该报错，只是没有可迁的行了
	if retry, err := commentService.MigrateKey(testCtx, user.UID, "old-key", "new-key"); err != nil || retry != 0 {
		t.Errorf("retry MigrateKey = (%d, %v), want (0, nil)", retry, err)
	}
}

// TestCommentService_MigrateKey_SameKey 新旧相同返回 A0305。
func TestCommentService_MigrateKey_SameKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := testComments
	_, err := commentService.MigrateKey(testCtx, 1, "same-key", "same-key")
	if err != model.ErrCommentKeySame {
		t.Errorf("Expected ErrCommentKeySame, got %v", err)
	}
	// 空→空归为「键不合规」而不是「新旧相同」：空目标键是黑洞键，任何 scope 下
	// 都是缺参数，判成 A0305 会把人引向「换个不同的键」而不是「把键传全」。
	// 公开端点走不到这里（new_key 的 binding required 先挡），此断言钉的是模块自身。
	if _, err := commentService.MigrateKey(testCtx, 1, "", ""); !errors.Is(err, model.ErrCommentKeyInvalid) {
		t.Errorf("Expected empty target key to be ErrCommentKeyInvalid, got %v", err)
	}
}

// TestCommentService_MigrateKey_BookLevelToChapter 旧键为空 = 把本人未换键的历史行收进真实桶。
//
// M2 之前这些行就是「书籍级评论」（没有 chapter_url 可用），换轨后它们统一表现为
// comment_key 为空。合并书籍时客户端先做这一步，否则会留下一批谁也读不回来的孤儿。
func TestCommentService_MigrateKey_BookLevelToChapter(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "mig_book@example.com", "password123")
	commentService := testComments

	seedLegacyComment(t, user.UID, "b1", "", "")
	seedLegacyComment(t, user.UID, "b2", "", "")
	commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{Content: "c1", CommentKey: "key-a"})

	count, err := commentService.MigrateKey(testCtx, user.UID, "", "ch-1")
	if err != nil {
		t.Fatalf("MigrateKey with empty old key failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 unkeyed comments migrated, got %d", count)
	}

	result, _ := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{"ch-1"}, Page: 1, PageSize: 10})
	if result.Total != 2 {
		t.Errorf("Expected 2 under ch-1, got %d", result.Total)
	}
	if other, _ := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{"key-a"}, Page: 1, PageSize: 10}); other.Total != 1 {
		t.Errorf("key-a should be untouched, got %d", other.Total)
	}
}

// TestCommentService_MigrateKey_UserIsolation 只迁本人，空键收拢也不例外。
func TestCommentService_MigrateKey_UserIsolation(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user1 := serviceRegister(t, authService, "mig_u1@example.com", "password123")
	user2 := serviceRegister(t, authService, "mig_u2@example.com", "password123")
	commentService := testComments

	commentService.Create(testCtx, user1.UID, &model.CreateCommentRequest{Content: "u1c1", CommentKey: "shared-key"})
	seedLegacyComment(t, user1.UID, "u1-unkeyed", "", "")
	commentService.Create(testCtx, user2.UID, &model.CreateCommentRequest{Content: "u2c1", CommentKey: "shared-key"})
	seedLegacyComment(t, user2.UID, "u2-unkeyed", "", "")

	// user1 迁移：只影响自己的 1 条
	count, err := commentService.MigrateKey(testCtx, user1.UID, "shared-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey user1 failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 migrated for user1, got %d", count)
	}

	// user2 的评论仍在旧键下
	result, _ := commentService.List(testCtx, model.CommentFilter{CommentKeys: []string{"shared-key"}, Page: 1, PageSize: 10})
	if result.Total != 1 || result.Items[0].User.UID != user2.UID {
		t.Errorf("Expected 1 under shared-key owned by user2, got %+v", result.Items)
	}

	// 空键收拢同样按 user_id 收窄：只能带走 user1 自己那条未换键的行
	swept, err := commentService.MigrateKey(testCtx, user1.UID, "", "ch-1")
	if err != nil || swept != 1 {
		t.Fatalf("empty-old-key sweep = (%d, %v), want (1, nil)", swept, err)
	}
	// 空键不是可查询的桶；直接读 user2 的行确认它仍未被改键。
	remaining, err := testComments.comments.FindAllByUserID(testCtx, user2.UID)
	if err != nil {
		t.Fatalf("FindAllByUserID: %v", err)
	}
	var unkeyed int
	for _, c := range remaining {
		if c.CommentKey == "" {
			unkeyed++
		}
	}
	if unkeyed != 1 {
		t.Errorf("user2's unkeyed row must stay unkeyed, got %d", unkeyed)
	}
}

// TestCommentService_MigrateKeys_Rules 改键规则只有一处实现，两个 scope 共用。
//
// migrate（本人）与 rehash（全员）的差别只在归属，规则本身必须一致地成立——
// 此前后台路径自己抄了一遍判重与长度，其中长度那份没有用例钉住。
func TestCommentService_MigrateKeys_Rules(t *testing.T) {
	long := strings.Repeat("k", model.MaxCommentKeyLength+1)

	cases := []struct {
		name     string
		scope    model.CommentKeyScope
		oldKey   string
		newKey   string
		wantErr  error
		wantText string
	}{
		{"本人范围允许空旧键", model.ScopeOfUser(1), "", "ck1:new", nil, ""},
		{"全局范围拒绝空旧键", model.ScopeAllUsers(), "", "ck1:new", model.ErrCommentKeyInvalid, "必须指定旧键"},
		{"目标键不得为空（黑洞键）", model.ScopeOfUser(1), "ck1:old", "", model.ErrCommentKeyInvalid, "目标聚合键不能为空"},
		{"目标键不得为空（全局）", model.ScopeAllUsers(), "ck1:old", "", model.ErrCommentKeyInvalid, "目标聚合键不能为空"},
		{"新旧相同回 A0305", model.ScopeOfUser(1), "ck1:same", "ck1:same", model.ErrCommentKeySame, ""},
		{"新旧相同回 A0305（全局）", model.ScopeAllUsers(), "ck1:same", "ck1:same", model.ErrCommentKeySame, ""},
		{"新键超长被拒", model.ScopeOfUser(1), "ck1:old", long, model.ErrCommentKeyInvalid, "最长"},
		{"旧键超长被拒", model.ScopeAllUsers(), long, "ck1:new", model.ErrCommentKeyInvalid, "最长"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestDB(t)
			defer cleanupTestDB(t)

			_, err := testComments.MigrateKeys(testCtx, tc.scope, tc.oldKey, tc.newKey)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantText != "" && !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("err text = %q, want it to contain %q", err.Error(), tc.wantText)
			}
		})
	}
}

// TestCommentService_MigrateKeys_ScopeIsTheOnlyDifference 同一次调用换 scope 即换归属。
func TestCommentService_MigrateKeys_ScopeIsTheOnlyDifference(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	mine := serviceRegister(t, testAuth, "scope-mine@example.com", "password123").UID
	theirs := serviceRegister(t, testAuth, "scope-theirs@example.com", "password123").UID
	for _, uid := range []uint{mine, theirs} {
		if _, err := testComments.Create(testCtx, uid, &model.CreateCommentRequest{
			Content: "c", CommentKey: "ck1:bad",
		}); err != nil {
			t.Fatalf("seed comment failed: %v", err)
		}
	}

	if _, err := testComments.MigrateKeys(testCtx, model.ScopeOfUser(mine), "ck1:bad", "ck1:fixed"); err != nil {
		t.Fatalf("user-scoped migrate failed: %v", err)
	}
	if listed, err := testComments.List(testCtx, model.CommentFilter{CommentKeys: []string{"ck1:bad"}, Page: 1, PageSize: 10}); err != nil || listed.Total != 1 {
		t.Errorf("他人行不应被本人迁移带走，ck1:bad 剩 %d 条", listed.Total)
	}

	if _, err := testComments.MigrateKeys(testCtx, model.ScopeAllUsers(), "ck1:bad", "ck1:fixed"); err != nil {
		t.Fatalf("global rehash failed: %v", err)
	}
	if listed, err := testComments.List(testCtx, model.CommentFilter{CommentKeys: []string{"ck1:bad"}, Page: 1, PageSize: 10}); err != nil || listed.Total != 0 {
		t.Errorf("全局改键应清空旧桶，剩 %d 条", listed.Total)
	}
	if listed, err := testComments.List(testCtx, model.CommentFilter{CommentKeys: []string{"ck1:fixed"}, Page: 1, PageSize: 10}); err != nil || listed.Total != 2 {
		t.Errorf("两个桶应合并到 ck1:fixed，得 %d 条", listed.Total)
	}
}
