package service

import (
	"ebook-server/model"
	"testing"
)

func TestCommentService_Create_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 先创建用户
	authService := NewAuthService()
	user := serviceRegister(t, authService, "commentuser@example.com", "password123")

	// 创建评论
	commentService := NewCommentService()
	createReq := &model.CreateCommentRequest{
		Content: "This is a test comment",
	}

	comment, err := commentService.Create(user.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	if comment.Content != createReq.Content {
		t.Errorf("Expected content '%s', got '%s'", createReq.Content, comment.Content)
	}

	if comment.UserID != user.UID {
		t.Errorf("Expected UserID %d, got %d", user.UID, comment.UserID)
	}
}

func TestCommentService_GetByID_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 创建用户和评论
	authService := NewAuthService()
	user := serviceRegister(t, authService, "getcommentuser@example.com", "password123")

	commentService := NewCommentService()
	createReq := &model.CreateCommentRequest{
		Content: "Comment to retrieve",
	}
	createdComment, err := commentService.Create(user.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// 获取评论
	comment, err := commentService.GetByID(createdComment.ID)
	if err != nil {
		t.Fatalf("Failed to get comment: %v", err)
	}

	if comment.ID != createdComment.ID {
		t.Errorf("Expected ID %d, got %d", createdComment.ID, comment.ID)
	}
}

func TestCommentService_GetByID_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := NewCommentService()

	_, err := commentService.GetByID(999999)
	if err != model.ErrCommentNotFound {
		t.Errorf("Expected ErrCommentNotFound, got %v", err)
	}
}

func TestCommentService_GetAll_Pagination(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := NewCommentService()

	// 创建一些评论
	authService := NewAuthService()
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
	authService := NewAuthService()
	user := serviceRegister(t, authService, "deletecommentuser@example.com", "password123")

	commentService := NewCommentService()
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

	// 验证删除
	_, err = commentService.GetByID(comment.ID)
	if err != model.ErrCommentNotFound {
		t.Errorf("Expected ErrCommentNotFound after deletion, got %v", err)
	}
}

func TestCommentService_Delete_NoPermission(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 创建两个用户
	authService := NewAuthService()
	user1 := serviceRegister(t, authService, "user1_delete@example.com", "password123")
	user2 := serviceRegister(t, authService, "user2_delete@example.com", "password123")

	// user1 创建评论
	commentService := NewCommentService()
	createReq := &model.CreateCommentRequest{
		Content: "Comment by user1",
	}
	comment, err := commentService.Create(user1.UID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// user2 尝试删除 user1 的评论
	err = commentService.Delete(comment.ID, user2.UID)
	if err != model.ErrNoPermission {
		t.Errorf("Expected ErrNoPermission, got %v", err)
	}
}

func TestCommentService_Delete_CommentNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	commentService := NewCommentService()

	err := commentService.Delete(999999, 1)
	if err != model.ErrCommentNotFound {
		t.Errorf("Expected ErrCommentNotFound, got %v", err)
	}
}

func TestCommentService_GetByUserID_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	user := serviceRegister(t, authService, "getbyuserid@example.com", "password123")

	commentService := NewCommentService()
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

	commentService := NewCommentService()
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

	authService := NewAuthService()
	user := serviceRegister(t, authService, "bounds@example.com", "password123")
	commentService := NewCommentService()

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
