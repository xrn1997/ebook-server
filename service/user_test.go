package service

import (
	"ebook-server/model"
	"testing"
)

func TestUserService_GetByID_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 先创建用户
	authService := testAuth
	user := serviceRegister(t, authService, "getbyiduser@example.com", "password123")

	// 获取用户
	userService := testUsers
	got, err := userService.GetByUID(user.UID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if got.Username != user.Username {
		t.Errorf("Expected username '%s', got '%s'", user.Username, got.Username)
	}
}

func TestUserService_GetByUID_UserNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userService := testUsers

	_, err := userService.GetByUID(999999)
	if err != model.ErrUserNotFound {
		t.Errorf("Expected ErrUserNotFound, got %v", err)
	}
}

func TestUserService_Update_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 创建用户
	authService := testAuth
	user := serviceRegister(t, authService, "old@example.com", "password123")

	// 更新用户（登录主标识不可变更，故只改展示用名与头像）
	userService := testUsers
	updateReq := &model.UpdateUserRequest{
		Username: "newuser",
		Avatar:   "https://example.com/new-avatar.jpg",
	}

	updatedUser, err := userService.Update(user.UID, updateReq)
	if err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}

	if updatedUser.Username != updateReq.Username {
		t.Errorf("Expected username '%s', got '%s'", updateReq.Username, updatedUser.Username)
	}

	// 邮箱应保持不变（ADR-0004）
	if updatedUser.Email != "old@example.com" {
		t.Errorf("Expected email to remain 'old@example.com', got '%s'", updatedUser.Email)
	}

	if updatedUser.Avatar != updateReq.Avatar {
		t.Errorf("Expected avatar '%s', got '%s'", updateReq.Avatar, updatedUser.Avatar)
	}
}

// TestUserService_Update_EmailImmutable 登录主标识不可变更（ADR-0004）
func TestUserService_Update_EmailImmutable(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "immutable@example.com", "password123")

	userService := testUsers
	_, err := userService.Update(user.UID, &model.UpdateUserRequest{
		Email: "another@example.com",
	})

	if err != model.ErrEmailImmutable {
		t.Errorf("Expected ErrEmailImmutable, got %v", err)
	}

	// 确认数据库中邮箱未被改动
	got, err := userService.GetByUID(user.UID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if got.Email != "immutable@example.com" {
		t.Errorf("Expected email unchanged, got '%s'", got.Email)
	}
}

// TestUserService_Update_EmailSameValue 传入与当前值相同的邮箱应幂等忽略
func TestUserService_Update_EmailSameValue(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "same@example.com", "password123")

	userService := testUsers
	updated, err := userService.Update(user.UID, &model.UpdateUserRequest{
		Email:    "same@example.com",
		Nickname: "新昵称",
	})

	if err != nil {
		t.Fatalf("Expected same-email update to succeed, got %v", err)
	}
	if updated.Nickname != "新昵称" {
		t.Errorf("Expected nickname to be updated, got '%s'", updated.Nickname)
	}
}

func TestUserService_Update_UserNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userService := testUsers

	updateReq := &model.UpdateUserRequest{
		Nickname: "新昵称",
	}

	_, err := userService.Update(999999, updateReq)
	if err != model.ErrUserNotFound {
		t.Errorf("Expected ErrUserNotFound, got %v", err)
	}
}

func TestUserService_Update_PartialUpdate(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 创建用户
	authService := testAuth
	user := serviceRegister(t, authService, "original@example.com", "password123")

	// 只更新头像
	userService := testUsers
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
