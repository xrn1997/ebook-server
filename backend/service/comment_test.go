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

// TestCommentService_Create_WithChapter 创建带章节归属的评论（ADR-0011）。
func TestCommentService_Create_WithChapter(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "chapter_user@example.com", "password123")

	commentService := testComments
	createReq := &model.CreateCommentRequest{
		Content:     "章节评论",
		ChapterURL:  "https://src.example.com/book/1/2.html",
		ChapterName: "第二章",
		BookName:    "天启之书",
	}
	comment, err := commentService.Create(user.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create chapter comment: %v", err)
	}

	if comment.ChapterURL != createReq.ChapterURL ||
		comment.ChapterName != createReq.ChapterName ||
		comment.BookName != createReq.BookName {
		t.Errorf("chapter fields not persisted: %+v", comment)
	}
}

// TestCommentService_GetByChapter 按章节聚合键查询（ADR-0011）。
func TestCommentService_GetByChapter(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "chapter_list@example.com", "password123")
	commentService := testComments

	urlA := "https://src.example.com/book/1/2.html"
	urlB := "https://src.example.com/book/1/3.html"
	// 章节 A 两条、章节 B 一条、书籍级一条
	for i := 0; i < 2; i++ {
		commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A", ChapterURL: urlA, BookName: "书A"})
	}
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "B", ChapterURL: urlB, BookName: "书B"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "通用"})

	result, err := commentService.GetByChapter(urlA, "", 1, 10)
	if err != nil {
		t.Fatalf("Failed to get chapter comments: %v", err)
	}
	if result.Total != 2 || len(result.Items) != 2 {
		t.Errorf("Expected 2 comments for chapter A, got total=%d len=%d", result.Total, len(result.Items))
	}

	// book_name 二次过滤：传错书名应过滤干净
	result, err = commentService.GetByChapter(urlA, "书B", 1, 10)
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

func TestCommentService_GetByChapterURLs_SingleKeyEquivalent(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "single_url@example.com", "password123")
	commentService := testComments

	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A1", ChapterURL: "key-a", BookName: "书A"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "A2", ChapterURL: "key-a", BookName: "书A"})

	// 单键通过 GetByChapterURLs 应等价于 GetByChapter
	multi, err := commentService.GetByChapterURLs([]string{"key-a"}, "书A", 1, 10)
	if err != nil {
		t.Fatalf("GetByChapterURLs single failed: %v", err)
	}
	single, err := commentService.GetByChapter("key-a", "书A", 1, 10)
	if err != nil {
		t.Fatalf("GetByChapter failed: %v", err)
	}
	if multi.Total != single.Total {
		t.Errorf("multi.Total=%d != single.Total=%d", multi.Total, single.Total)
	}
}

func TestCommentService_MigrateKey_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "migrate@example.com", "password123")
	commentService := testComments

	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "c1", ChapterURL: "old-key"})
	commentService.Create(user.UID, &model.CreateCommentRequest{Content: "c2", ChapterURL: "old-key"})

	count, err := commentService.MigrateKey(user.UID, "old-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 migrated, got %d", count)
	}

	// 新键下应有 2 条
	result, _ := commentService.GetByChapterURLs([]string{"new-key"}, "", 1, 10)
	if result.Total != 2 {
		t.Errorf("Expected 2 under new-key, got %d", result.Total)
	}
}

func TestCommentService_MigrateKey_SameKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := testComments
	_, err := commentService.MigrateKey(1, "same-key", "same-key")
	if err != model.ErrCommentKeySame {
		t.Errorf("Expected ErrCommentKeySame, got %v", err)
	}
}

func TestCommentService_MigrateKey_UserIsolation(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user1 := serviceRegister(t, authService, "mig_u1@example.com", "password123")
	user2 := serviceRegister(t, authService, "mig_u2@example.com", "password123")
	commentService := testComments

	commentService.Create(user1.UID, &model.CreateCommentRequest{Content: "u1c1", ChapterURL: "shared-key"})
	commentService.Create(user2.UID, &model.CreateCommentRequest{Content: "u2c1", ChapterURL: "shared-key"})

	// user1 迁移：只影响自己的 1 条
	count, err := commentService.MigrateKey(user1.UID, "shared-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey user1 failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 migrated for user1, got %d", count)
	}

	// user2 的评论仍在旧键下
	result, _ := commentService.GetByChapter("shared-key", "", 1, 10)
	if result.Total != 1 {
		t.Errorf("Expected 1 under shared-key for user2, got %d", result.Total)
	}
}
