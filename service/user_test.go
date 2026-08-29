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
				Secret:    "test-secret",
				ExpireMin: 1,
			},
		}
	}
	config.AppConfig.Database.Path = ":memory:"

	if err := database.Init(); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}

	testDB = database.GetDB()
	testDB.AutoMigrate(&model.User{}, &model.Comment{}, &model.OperationLog{}, &model.RefreshToken{})
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
	user := serviceRegister(t, authService, "getbyiduser@example.com", "password123")

	// 获取用户
	userService := NewUserService()
	got, err := userService.GetByUID(user.UID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if got.Username != user.Username {
		t.Errorf("Expected username '%s', got '%s'", user.Username, got.Username)
	}
}

func TestUserService_GetByUID_UserNotFound(t *testing.T) {
	setupUserTestDB(t)
	defer cleanupUserTestDB(t)

	userService := NewUserService()

	_, err := userService.GetByUID(999999)
	if err != model.ErrUserNotFound {
		t.Errorf("Expected ErrUserNotFound, got %v", err)
	}
}

func TestUserService_Update_Success(t *testing.T) {
	setupUserTestDB(t)
	defer cleanupUserTestDB(t)

	// 创建用户
	authService := NewAuthService()
	user := serviceRegister(t, authService, "old@example.com", "password123")

	// 更新用户
	userService := NewUserService()
	updateReq := &model.UpdateUserRequest{
		Username: "newuser",
		Email:    "new@example.com",
		Avatar:   "https://example.com/new-avatar.jpg",
	}

	updatedUser, err := userService.Update(user.UID, updateReq)
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
	user := serviceRegister(t, authService, "original@example.com", "password123")

	// 只更新头像
	userService := NewUserService()
	updateReq := &model.UpdateUserRequest{
		Avatar: "https://example.com/avatar.jpg",
	}

	updatedUser, err := userService.Update(user.UID, updateReq)
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
