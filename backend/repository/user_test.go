package repository

import (
	"ebook-server/model"
	"testing"
	"time"
)

func TestUserRepository_Create(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "test@example.com",
		Password: "hashedpassword",
		Username: "testuser",
		Nickname: "testuser",
	}

	if err := repo.Create(user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user.UID == 0 {
		t.Error("Expected non-zero UID after creation")
	}
}

func TestUserRepository_FindByEmail_Found(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "find@example.com",
		Password: "hashedpassword",
		Username: "finduser",
		Nickname: "finduser",
	}
	repo.Create(user)

	found, err := repo.FindByEmail("find@example.com")
	if err != nil {
		t.Fatalf("Failed to find user: %v", err)
	}
	if found.Email != "find@example.com" {
		t.Errorf("Expected email 'find@example.com', got '%s'", found.Email)
	}
}

func TestUserRepository_FindByEmail_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	_, err := repo.FindByEmail("nonexistent@example.com")
	if !IsRecordNotFound(err) {
		t.Errorf("Expected ErrRecordNotFound, got %v", err)
	}
}

func TestUserRepository_FindByUID_Found(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "uid@example.com",
		Password: "hashedpassword",
		Username: "uiduser",
		Nickname: "uiduser",
	}
	repo.Create(user)

	found, err := repo.FindByUID(user.UID)
	if err != nil {
		t.Fatalf("Failed to find user: %v", err)
	}
	if found.UID != user.UID {
		t.Errorf("Expected UID %d, got %d", user.UID, found.UID)
	}
}

func TestUserRepository_FindByUID_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	_, err := repo.FindByUID(999999)
	if !IsRecordNotFound(err) {
		t.Errorf("Expected ErrRecordNotFound, got %v", err)
	}
}

func TestUserRepository_Update(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "update@example.com",
		Password: "hashedpassword",
		Username: "before",
		Nickname: "before",
	}
	repo.Create(user)

	user.Username = "after"
	user.Avatar = "https://example.com/avatar.jpg"
	if err := repo.Update(user); err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}

	found, _ := repo.FindByUID(user.UID)
	if found.Username != "after" {
		t.Errorf("Expected username 'after', got '%s'", found.Username)
	}
	if found.Avatar != "https://example.com/avatar.jpg" {
		t.Errorf("Expected avatar updated, got '%s'", found.Avatar)
	}
}

func TestUserRepository_ExistsByEmail_True(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "exists@example.com",
		Password: "hashedpassword",
		Username: "existsuser",
		Nickname: "existsuser",
	}
	repo.Create(user)

	exists, err := repo.ExistsByEmail("exists@example.com")
	if err != nil {
		t.Fatalf("Failed to check existence: %v", err)
	}
	if !exists {
		t.Error("Expected user to exist")
	}
}

func TestUserRepository_ExistsByEmail_False(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	exists, err := repo.ExistsByEmail("ghost@example.com")
	if err != nil {
		t.Fatalf("Failed to check existence: %v", err)
	}
	if exists {
		t.Error("Expected user not to exist")
	}
}

func TestUserRepository_LoginAttemptsFields(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "lock@example.com",
		Password: "hashedpassword",
		Username: "lockuser",
		Nickname: "lockuser",
	}
	repo.Create(user)

	// 设置锁定状态
	user.LoginAttempts = 3
	until := time.Now().Add(15 * time.Minute)
	user.LockedUntil = &until
	repo.Update(user)

	found, _ := repo.FindByUID(user.UID)
	if found.LoginAttempts != 3 {
		t.Errorf("Expected LoginAttempts 3, got %d", found.LoginAttempts)
	}
	if found.LockedUntil == nil {
		t.Error("Expected LockedUntil to be set")
	}
}
