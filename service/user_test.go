package service

import (
	"ebook-server/config"
	"ebook-server/model"
	"ebook-server/pkg/database"
	"testing"
)

func setupUserTestDB(t *testing.T) {
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

func cleanupUserTestDB(t *testing.T) {
	t.Helper()
	if database.DB != nil {
		sqlDB, _ := database.DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		database.DB = nil
	}
}

func TestUserService_GetByID_Success(t *testing.T) {
	setupUserTestDB(t)
	defer cleanupUserTestDB(t)

	// 先创建用户
	authService := NewAuthService()
	req := &model.RegisterRequest{
		Username: "getbyiduser",
		Password: "password123",
	}
	createdUser, err := authService.Register(req)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	// 获取用户
	userService := NewUserService()
	user, err := userService.GetByID(createdUser.ID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if user.Username != "getbyiduser" {
		t.Errorf("Expected username 'getbyiduser', got '%s'", user.Username)
	}
}

func TestUserService_GetByID_UserNotFound(t *testing.T) {
	setupUserTestDB(t)
	defer cleanupUserTestDB(t)

	userService := NewUserService()

	_, err := userService.GetByID(999999)
	if err != model.ErrUserNotFound {
		t.Errorf("Expected ErrUserNotFound, got %v", err)
	}
}

func TestUserService_Update_Success(t *testing.T) {
	setupUserTestDB(t)
	defer cleanupUserTestDB(t)

	// 创建用户
	authService := NewAuthService()
	registerReq := &model.RegisterRequest{
		Username: "updateuser",
		Password: "password123",
		Email:    "old@example.com",
	}
	user, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	// 更新用户
	userService := NewUserService()
	updateReq := &model.UpdateUserRequest{
		Email:  "new@example.com",
		Avatar: "https://example.com/new-avatar.jpg",
	}

	updatedUser, err := userService.Update(user.ID, updateReq)
	if err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}

	if updatedUser.Email != updateReq.Email {
		t.Errorf("Expected email '%s', got '%s'", updateReq.Email, updatedUser.Email)
	}

	if updatedUser.Avatar != updateReq.Avatar {
		t.Errorf("Expected avatar '%s', got '%s'", updateReq.Avatar, updatedUser.Avatar)
	}
}

func TestUserService_Update_UserNotFound(t *testing.T) {
	setupUserTestDB(t)
	defer cleanupUserTestDB(t)

	userService := NewUserService()

	updateReq := &model.UpdateUserRequest{
		Email: "new@example.com",
	}

	_, err := userService.Update(999999, updateReq)
	if err != model.ErrUserNotFound {
		t.Errorf("Expected ErrUserNotFound, got %v", err)
	}
}

func TestUserService_Update_PartialUpdate(t *testing.T) {
	setupUserTestDB(t)
	defer cleanupUserTestDB(t)

	// 创建用户
	authService := NewAuthService()
	registerReq := &model.RegisterRequest{
		Username: "partialupdateuser",
		Password: "password123",
		Email:    "original@example.com",
	}
	user, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	// 只更新头像
	userService := NewUserService()
	updateReq := &model.UpdateUserRequest{
		Avatar: "https://example.com/avatar.jpg",
	}

	updatedUser, err := userService.Update(user.ID, updateReq)
	if err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}

	// 邮箱应该保持不变
	if updatedUser.Email != "original@example.com" {
		t.Errorf("Expected email to remain 'original@example.com', got '%s'", updatedUser.Email)
	}

	// 头像应该被更新
	if updatedUser.Avatar != updateReq.Avatar {
		t.Errorf("Expected avatar '%s', got '%s'", updateReq.Avatar, updatedUser.Avatar)
	}
}
