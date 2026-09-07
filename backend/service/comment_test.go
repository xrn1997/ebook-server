package service

import (
	"ebook-server/model"
	"testing"
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
		Content: "This is a test comment",
	}

	comment, err := commentService.Create(user.UID, createReq)
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
	comment, err := commentService.Create(user.UID, createReq)
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

// ── M2 主读路径：GetByCommentKeys ────────────────────────────────────────

// TestCommentService_GetByCommentKeys_SingleKey 单键查询即精确匹配该键。
//
// 键由客户端派生、服务端不解释，所以断言只看命中集合与作者视图，不校验键形态。
func TestCommentService_GetByCommentKeys_SingleKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "ck_single@example.com", "password123")
	commentService := testComments

	keyA := "ck1:aaaa#1"
	keyB := "ck1:aaaa#2"
	for i := 0; i < 2; i++ {
		commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A", CommentKey: keyA, BookName: "书A"})
	}
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "B", CommentKey: keyB, BookName: "书A"})

	result, err := commentService.GetByCommentKeys([]string{keyA}, 1, 10)
	if err != nil {
		t.Fatalf("GetByCommentKeys failed: %v", err)
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

// TestCommentService_GetByCommentKeys_Union 多键返回并集（跨书源合并同一作品）。
func TestCommentService_GetByCommentKeys_Union(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "ck_union@example.com", "password123")
	commentService := testComments

	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A1", CommentKey: "ck1:a#1"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A2", CommentKey: "ck1:a#1"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "B1", CommentKey: "ck1:b#1"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "C1", CommentKey: "ck1:c#1"})

	result, err := commentService.GetByCommentKeys([]string{"ck1:a#1", "ck1:b#1"}, 1, 10)
	if err != nil {
		t.Fatalf("GetByCommentKeys union failed: %v", err)
	}
	if result.Total != 3 || len(result.Items) != 3 {
		t.Errorf("union = total %d len %d, want 3/3", result.Total, len(result.Items))
	}
}

// TestCommentService_GetByCommentKeys_Pagination 并集分页并把分页参数归一化。
func TestCommentService_GetByCommentKeys_Pagination(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "ck_pg@example.com", "password123")
	commentService := testComments

	for i := 0; i < 5; i++ {
		commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A", CommentKey: "ck1:a"})
	}
	for i := 0; i < 3; i++ {
		commentService.Create(user.UID, &model.CreateCommentRequest{Content: "B", CommentKey: "ck1:b"})
	}

	keys := []string{"ck1:a", "ck1:b"}
	page1, err := commentService.GetByCommentKeys(keys, 1, 3)
	if err != nil {
		t.Fatalf("GetByCommentKeys page 1 failed: %v", err)
	}
	if page1.Total != 8 || len(page1.Items) != 3 {
		t.Errorf("page 1 = total %d len %d, want 8/3", page1.Total, len(page1.Items))
	}
	if page1.Page != 1 || page1.PageSize != 3 {
		t.Errorf("page 1 echo = (%d,%d), want (1,3)", page1.Page, page1.PageSize)
	}

	// 末页只剩 2 条
	page3, _ := commentService.GetByCommentKeys(keys, 3, 3)
	if len(page3.Items) != 2 || page3.Total != 8 {
		t.Errorf("page 3 = len %d total %d, want 2/8", len(page3.Items), page3.Total)
	}

	// page/page_size 非法值归一化为 1/10（全站列表接口的统一约定）
	normalized, _ := commentService.GetByCommentKeys(keys, 0, 0)
	if normalized.Page != 1 || normalized.PageSize != 10 {
		t.Errorf("normalized = (%d,%d), want (1,10)", normalized.Page, normalized.PageSize)
	}
}

// TestCommentService_GetByCommentKeys_SkipsUnkeyedRows 空键不属于任何桶。
//
// 空 comment_key = 尚未换键的历史行。它们必须从非空键过滤里消失，否则客户端一旦
// 拼错键，就会把无主评论当成自己章节的内容显示出来。
func TestCommentService_GetByCommentKeys_SkipsUnkeyedRows(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "ck_unkeyed@example.com", "password123")
	commentService := testComments

	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "无键旧评"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "有键", CommentKey: "ck1:a#1"})

	result, err := commentService.GetByCommentKeys([]string{"ck1:a#1"}, 1, 10)
	if err != nil {
		t.Fatalf("GetByCommentKeys failed: %v", err)
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
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "旧行", ChapterURL: legacyURL})

	legacy, err := commentService.GetByChapterURLs([]string{legacyURL}, "", 1, 10)
	if err != nil {
		t.Fatalf("GetByChapterURLs failed: %v", err)
	}
	if legacy.Total != 1 || legacy.Items[0].CommentKey != "" {
		t.Errorf("legacy row = %+v, want 1 条且 comment_key 为空", legacy.Items)
	}

	// 同一个字符串不是新键：按 comment_key 查必须为空
	viaKey, err := commentService.GetByCommentKeys([]string{legacyURL}, 1, 10)
	if err != nil {
		t.Fatalf("GetByCommentKeys failed: %v", err)
	}
	if viaKey.Total != 0 {
		t.Errorf("chapter_url value must not match comment_key, got %d", viaKey.Total)
	}
}

// TestCommentService_GetByChapterURLs_SingleKey 已废弃路径：单键即精确匹配（ADR-0011）。
//
// M2 后聚合键换成 comment_key，这里守的是「旧客户端带 chapter_url + book_name 仍能查到」
// 这条兼容承诺，不是主读路径（主路径见 GetByCommentKeys）。
func TestCommentService_GetByChapterURLs_SingleKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "chapter_list@example.com", "password123")
	commentService := testComments

	urlA := "https://src.example.com/book/1/2.html"
	urlB := "https://src.example.com/book/1/3.html"
	// 章节 A 两条、章节 B 一条、未换键（chapter_url 为空）一条
	for i := 0; i < 2; i++ {
		commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A", ChapterURL: urlA, BookName: "书A"})
	}
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "B", ChapterURL: urlB, BookName: "书B"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "通用"})

	result, err := commentService.GetByChapterURLs([]string{urlA}, "", 1, 10)
	if err != nil {
		t.Fatalf("Failed to get chapter comments: %v", err)
	}
	if result.Total != 2 || len(result.Items) != 2 {
		t.Errorf("Expected 2 comments for chapter A, got total=%d len=%d", result.Total, len(result.Items))
	}

	// book_name 二次过滤：传错书名应过滤干净
	result, err = commentService.GetByChapterURLs([]string{urlA}, "书B", 1, 10)
	if err != nil {
		t.Fatalf("Failed to get filtered comments: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("Expected 0 comments for wrong book filter, got %d", result.Total)
	}
}

// TestCommentService_GetByBook 按书名单独过滤（ADR-0011：book_name 不依赖 chapter_url）。
func TestCommentService_GetByBook(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "book_filter@example.com", "password123")
	commentService := testComments

	// 书A 两个不同章节 + 书B 一条 + 无章节一条
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A1", ChapterURL: "https://s/book/1/1.html", BookName: "书A"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A2", ChapterURL: "https://s/book/1/2.html", BookName: "书A"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "B", ChapterURL: "https://s/book/2/1.html", BookName: "书B"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "无章节"})

	result, err := commentService.GetByBook("书A", 1, 10)
	if err != nil {
		t.Fatalf("Failed to get book comments: %v", err)
	}
	if result.Total != 2 || len(result.Items) != 2 {
		t.Errorf("Expected 2 comments for 书A, got total=%d len=%d", result.Total, len(result.Items))
	}
}

func TestCommentService_GetAll_Pagination(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := testComments

	// 创建一些评论
	authService := testAuth
	user := serviceRegister(t, authService, "pagination_user@example.com", "password123")
	uid := user.UID

	for i := 0; i < 15; i++ {
		commentService.Create(uid, &model.CreateCommentRequest{
			Content: "Comment content",
		})
	}

	// 获取第一页
	result1, err := commentService.GetAll(1, 5)
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
	result2, err := commentService.GetAll(2, 5)
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
		Content: "Comment to delete",
	}
	comment, err := commentService.Create(user.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// 删除评论
	err = commentService.Delete(comment.ID, user.UID)
	if err != nil {
		t.Fatalf("Failed to delete comment: %v", err)
	}

	// 验证删除：软删后全局列表不再包含该评论
	list, err := commentService.GetAll(1, 100)
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
		Content: "Comment by user1",
	}
	comment, err := commentService.Create(user1.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// user2 尝试删除 user1 的评论
	err = commentService.Delete(comment.ID, user2.UID)
	if err != model.ErrCommentNotOwner {
		t.Errorf("Expected ErrCommentNotOwner, got %v", err)
	}
}

func TestCommentService_Delete_CommentNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := testComments

	err := commentService.Delete(999999, 1)
	if err != model.ErrCommentNotFound {
		t.Errorf("Expected ErrCommentNotFound, got %v", err)
	}
}

func TestCommentService_GetByUserID_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "getbyuserid@example.com", "password123")

	commentService := testComments
	for i := 0; i < 5; i++ {
		commentService.Create(user.UID, &model.CreateCommentRequest{Content: "Comment"})
	}

	// 创建另一个用户的评论
	other := serviceRegister(t, authService, "other@example.com", "password123")
	commentService.Create(other.UID, &model.CreateCommentRequest{Content: "Other comment"})

	result, err := commentService.GetByUserID(user.UID, 1, 10)
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

func TestCommentService_GetByUserID_Empty(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := testComments
	result, err := commentService.GetByUserID(999, 1, 10)
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

func TestCommentService_GetAll_PaginationBoundaries(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "bounds@example.com", "password123")
	commentService := testComments

	for i := 0; i < 3; i++ {
		commentService.Create(user.UID, &model.CreateCommentRequest{Content: "Comment"})
	}

	// page=0 应修正为 1
	result, err := commentService.GetAll(0, 10)
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if result.Page != 1 {
		t.Errorf("Expected Page 1 for page=0, got %d", result.Page)
	}

	// page=-1 应修正为 1
	result, err = commentService.GetAll(-1, 10)
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if result.Page != 1 {
		t.Errorf("Expected Page 1 for page=-1, got %d", result.Page)
	}

	// pageSize=0 应修正为 10
	result, err = commentService.GetAll(1, 0)
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=0, got %d", result.PageSize)
	}

	// pageSize=101 应修正为 10
	result, err = commentService.GetAll(1, 101)
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=101, got %d", result.PageSize)
	}
}

func TestCommentService_GetByChapterURLs_Union(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "multi_url@example.com", "password123")
	commentService := testComments

	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A1", ChapterURL: "key-a", BookName: "书A"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A2", ChapterURL: "key-a", BookName: "书A"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "B1", ChapterURL: "key-b", BookName: "书A"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "C1", ChapterURL: "key-c", BookName: "书B"})

	result, err := commentService.GetByChapterURLs([]string{"key-a", "key-b"}, "", 1, 10)
	if err != nil {
		t.Fatalf("GetByChapterURLs failed: %v", err)
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

	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "c1", CommentKey: "old-key"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "c2", CommentKey: "old-key"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "c3", CommentKey: "other-key"})

	count, err := commentService.MigrateKey(user.UID, "old-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 migrated, got %d", count)
	}

	// 新键下应有 2 条
	result, _ := commentService.GetByCommentKeys([]string{"new-key"}, 1, 10)
	if result.Total != 2 {
		t.Errorf("Expected 2 under new-key, got %d", result.Total)
	}
	// 旧键清空，邻键不受影响
	if old, _ := commentService.GetByCommentKeys([]string{"old-key"}, 1, 10); old.Total != 0 {
		t.Errorf("old-key should be empty, got %d", old.Total)
	}
	if other, _ := commentService.GetByCommentKeys([]string{"other-key"}, 1, 10); other.Total != 1 {
		t.Errorf("other-key should be untouched, got %d", other.Total)
	}
	// 幂等：客户端重试不该报错，只是没有可迁的行了
	if retry, err := commentService.MigrateKey(user.UID, "old-key", "new-key"); err != nil || retry != 0 {
		t.Errorf("retry MigrateKey = (%d, %v), want (0, nil)", retry, err)
	}
}

// TestCommentService_MigrateKey_SameKey 新旧相同返回 A0305。
func TestCommentService_MigrateKey_SameKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := testComments
	_, err := commentService.MigrateKey(1, "same-key", "same-key")
	if err != model.ErrCommentKeySame {
		t.Errorf("Expected ErrCommentKeySame, got %v", err)
	}
	// 两键同时为空也是「相同」：那是「尚未换键」这个合法状态，
	// 空→空什么也不改变，与其静默返回 0 不如回 A0305 让客户端发现传错了。
	if _, err := commentService.MigrateKey(1, "", ""); err != model.ErrCommentKeySame {
		t.Errorf("Expected empty-empty to be ErrCommentKeySame, got %v", err)
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

	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "b1"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "b2", CommentKey: ""})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "c1", CommentKey: "key-a"})

	count, err := commentService.MigrateKey(user.UID, "", "ch-1")
	if err != nil {
		t.Fatalf("MigrateKey with empty old key failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 unkeyed comments migrated, got %d", count)
	}

	result, _ := commentService.GetByCommentKeys([]string{"ch-1"}, 1, 10)
	if result.Total != 2 {
		t.Errorf("Expected 2 under ch-1, got %d", result.Total)
	}
	if other, _ := commentService.GetByCommentKeys([]string{"key-a"}, 1, 10); other.Total != 1 {
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

	commentService.Create(user1.UID, &model.CreateCommentRequest{Content: "u1c1", CommentKey: "shared-key"})
	commentService.Create(user1.UID, &model.CreateCommentRequest{Content: "u1-unkeyed"})
	commentService.Create(user2.UID, &model.CreateCommentRequest{Content: "u2c1", CommentKey: "shared-key"})
	commentService.Create(user2.UID, &model.CreateCommentRequest{Content: "u2-unkeyed"})

	// user1 迁移：只影响自己的 1 条
	count, err := commentService.MigrateKey(user1.UID, "shared-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey user1 failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 migrated for user1, got %d", count)
	}

	// user2 的评论仍在旧键下
	result, _ := commentService.GetByCommentKeys([]string{"shared-key"}, 1, 10)
	if result.Total != 1 || result.Items[0].User.UID != user2.UID {
		t.Errorf("Expected 1 under shared-key owned by user2, got %+v", result.Items)
	}

	// 空键收拢同样按 user_id 收窄：只能带走 user1 自己那条未换键的行
	swept, err := commentService.MigrateKey(user1.UID, "", "ch-1")
	if err != nil || swept != 1 {
		t.Fatalf("empty-old-key sweep = (%d, %v), want (1, nil)", swept, err)
	}
	remaining, _ := commentService.GetByCommentKeys([]string{""}, 1, 10)
	if remaining.Total != 1 || remaining.Items[0].User.UID != user2.UID {
		t.Errorf("user2's unkeyed row must stay unkeyed, got %+v", remaining.Items)
	}
}
