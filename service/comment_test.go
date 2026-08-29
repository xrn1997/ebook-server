package service

import (
	"ebook-server/config"
	"ebook-server/model"
	"ebook-server/pkg/database"
	"testing"
)

func setupCommentTestDB(t *testing.T) {
	t.Helper()

	// 重置数据库连接
	if database.DB != nil {
		sqlDB, _ := database.DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		database.DB = nil
	}

	// 确保配置已初始化
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{
			JWT: config.JWTConfig{
				Secret:     "test-secret",
				ExpireHour: 1,
			},
		}
	}
	config.AppConfig.Database.Path = ":memory:"

	if err := database.Init(); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}

	testDB = database.GetDB()
	testDB.AutoMigrate(&model.User{}, &model.Comment{}, &model.OperationLog{})
}

func cleanupCommentTestDB(t *testing.T) {
	t.Helper()
	if database.DB != nil {
		sqlDB, _ := database.DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		database.DB = nil
	}
}

func TestCommentService_Create_Success(t *testing.T) {
	setupCommentTestDB(t)
	defer cleanupCommentTestDB(t)

	// 先创建用户
	authService := NewAuthService()
	registerReq := &model.RegisterRequest{
		Username: "commentuser",
		Password: "password123",
	}
	user, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	// 创建评论
	commentService := NewCommentService()
	createReq := &model.CreateCommentRequest{
		Content: "This is a test comment",
	}

	comment, err := commentService.Create(user.ID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	if comment.Content != createReq.Content {
		t.Errorf("Expected content '%s', got '%s'", createReq.Content, comment.Content)
	}

	if comment.UserID != user.ID {
		t.Errorf("Expected UserID %d, got %d", user.ID, comment.UserID)
	}
}

func TestCommentService_GetByID_Success(t *testing.T) {
	setupCommentTestDB(t)
	defer cleanupCommentTestDB(t)

	// 创建用户和评论
	authService := NewAuthService()
	registerReq := &model.RegisterRequest{
		Username: "getcommentuser",
		Password: "password123",
	}
	user, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	commentService := NewCommentService()
	createReq := &model.CreateCommentRequest{
		Content: "Comment to retrieve",
	}
	createdComment, err := commentService.Create(user.ID, createReq)
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
	setupCommentTestDB(t)
	defer cleanupCommentTestDB(t)

	commentService := NewCommentService()

	_, err := commentService.GetByID(999999)
	if err != model.ErrCommentNotFound {
		t.Errorf("Expected ErrCommentNotFound, got %v", err)
	}
}

func TestCommentService_GetAll_Pagination(t *testing.T) {
	setupCommentTestDB(t)
	defer cleanupCommentTestDB(t)

	commentService := NewCommentService()

	// 创建一些评论
	authService := NewAuthService()
	user, _ := authService.Register(&model.RegisterRequest{
		Username: "pagination_user",
		Password: "password123",
	})

	for i := 0; i < 15; i++ {
		commentService.Create(user.ID, &model.CreateCommentRequest{
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
	setupCommentTestDB(t)
	defer cleanupCommentTestDB(t)

	// 创建用户和评论
	authService := NewAuthService()
	registerReq := &model.RegisterRequest{
		Username: "deletecommentuser",
		Password: "password123",
	}
	user, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	commentService := NewCommentService()
	createReq := &model.CreateCommentRequest{
		Content: "Comment to delete",
	}
	comment, err := commentService.Create(user.ID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// 删除评论
	err = commentService.Delete(comment.ID, user.ID)
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
	setupCommentTestDB(t)
	defer cleanupCommentTestDB(t)

	// 创建两个用户
	authService := NewAuthService()

	user1Req := &model.RegisterRequest{
		Username: "user1_delete",
		Password: "password123",
	}
	user1, err := authService.Register(user1Req)
	if err != nil {
		t.Fatalf("Failed to register user1: %v", err)
	}

	user2Req := &model.RegisterRequest{
		Username: "user2_delete",
		Password: "password123",
	}
	user2, err := authService.Register(user2Req)
	if err != nil {
		t.Fatalf("Failed to register user2: %v", err)
	}

	// user1 创建评论
	commentService := NewCommentService()
	createReq := &model.CreateCommentRequest{
		Content: "Comment by user1",
	}
	comment, err := commentService.Create(user1.ID, createReq)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// user2 尝试删除 user1 的评论
	err = commentService.Delete(comment.ID, user2.ID)
	if err != model.ErrNoPermission {
		t.Errorf("Expected ErrNoPermission, got %v", err)
	}
}

func TestCommentService_Delete_CommentNotFound(t *testing.T) {
	setupCommentTestDB(t)
	defer cleanupCommentTestDB(t)

	commentService := NewCommentService()

	err := commentService.Delete(999999, 1)
	if err != model.ErrCommentNotFound {
		t.Errorf("Expected ErrCommentNotFound, got %v", err)
	}
}
