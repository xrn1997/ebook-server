package repository

import (
	"ebook-server/model"
	"testing"
)

func TestCommentRepository_Create(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	// 先创建用户
	userRepo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "comment@example.com",
		Password: "hashedpassword",
		Username: "commentuser",
		Nickname: "commentuser",
	}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	comment := &model.Comment{
		UserID:  user.UID,
		Content: "This is a test comment",
	}

	if err := repo.Create(comment); err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}
	if comment.ID == 0 {
		t.Error("Expected non-zero ID after creation")
	}
}

func TestCommentRepository_FindByID_Found(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "findcomment@example.com",
		Password: "hashedpassword",
		Username: "findcommentuser",
		Nickname: "findcommentuser",
	}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	comment := &model.Comment{
		UserID:  user.UID,
		Content: "Find me",
	}
	repo.Create(comment)

	found, err := repo.FindByID(comment.ID)
	if err != nil {
		t.Fatalf("Failed to find comment: %v", err)
	}
	if found.Content != "Find me" {
		t.Errorf("Expected content 'Find me', got '%s'", found.Content)
	}
	// 验证 Preload User
	if found.User.Email != "findcomment@example.com" {
		t.Errorf("Expected preloaded user email 'findcomment@example.com', got '%s'", found.User.Email)
	}
}

func TestCommentRepository_FindByID_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewCommentRepository(testDB)
	_, err := repo.FindByID(999999)
	if !IsRecordNotFound(err) {
		t.Errorf("Expected ErrRecordNotFound, got %v", err)
	}
}

func TestCommentRepository_FindByUserID(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "usercomments@example.com",
		Password: "hashedpassword",
		Username: "usercomments",
		Nickname: "usercomments",
	}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	for i := 0; i < 5; i++ {
		repo.Create(&model.Comment{
			UserID:  user.UID,
			Content: "Comment",
		})
	}

	comments, total, err := repo.FindByUserID(user.UID, 1, 3)
	if err != nil {
		t.Fatalf("Failed to find comments: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(comments) != 3 {
		t.Errorf("Expected 3 comments, got %d", len(comments))
	}
}

func TestCommentRepository_FindAllByUserID(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "exportuser@example.com",
		Password: "hashedpassword",
		Username: "exportuser",
		Nickname: "exportuser",
	}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	for i := 0; i < 5; i++ {
		repo.Create(&model.Comment{
			UserID:  user.UID,
			Content: "Export comment",
		})
	}

	comments, err := repo.FindAllByUserID(user.UID)
	if err != nil {
		t.Fatalf("Failed to find comments: %v", err)
	}
	if len(comments) != 5 {
		t.Errorf("Expected 5 comments, got %d", len(comments))
	}
}

func TestCommentRepository_FindAll(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "allcomments@example.com",
		Password: "hashedpassword",
		Username: "allcomments",
		Nickname: "allcomments",
	}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	for i := 0; i < 15; i++ {
		repo.Create(&model.Comment{
			UserID:  user.UID,
			Content: "Comment",
		})
	}

	// 第一页
	comments, total, err := repo.FindAll(1, 10)
	if err != nil {
		t.Fatalf("Failed to find comments: %v", err)
	}
	if total != 15 {
		t.Errorf("Expected total 15, got %d", total)
	}
	if len(comments) != 10 {
		t.Errorf("Expected 10 comments, got %d", len(comments))
	}

	// 第二页
	comments2, _, _ := repo.FindAll(2, 10)
	if len(comments2) != 5 {
		t.Errorf("Expected 5 comments on page 2, got %d", len(comments2))
	}
}

func TestCommentRepository_Delete(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "deletecomment@example.com",
		Password: "hashedpassword",
		Username: "deletecomment",
		Nickname: "deletecomment",
	}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	comment := &model.Comment{
		UserID:  user.UID,
		Content: "Delete me",
	}
	repo.Create(comment)

	if err := repo.Delete(comment.ID); err != nil {
		t.Fatalf("Failed to delete comment: %v", err)
	}

	_, err := repo.FindByID(comment.ID)
	if !IsRecordNotFound(err) {
		t.Errorf("Expected ErrRecordNotFound after deletion, got %v", err)
	}
}

func TestCommentRepository_CanDelete_Owner(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "owner@example.com",
		Password: "hashedpassword",
		Username: "owner",
		Nickname: "owner",
	}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	comment := &model.Comment{
		UserID:  user.UID,
		Content: "Owned comment",
	}
	repo.Create(comment)

	canDelete, err := repo.CanDelete(comment.ID, user.UID)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !canDelete {
		t.Error("Owner should be able to delete")
	}
}

func TestCommentRepository_CanDelete_NotOwner(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user1 := &model.User{
		Email:    "owner2@example.com",
		Password: "hashedpassword",
		Username: "owner2",
		Nickname: "owner2",
	}
	user2 := &model.User{
		Email:    "other@example.com",
		Password: "hashedpassword",
		Username: "other",
		Nickname: "other",
	}
	userRepo.Create(user1)
	userRepo.Create(user2)

	repo := NewCommentRepository(testDB)
	comment := &model.Comment{
		UserID:  user1.UID,
		Content: "Owned by user1",
	}
	repo.Create(comment)

	canDelete, err := repo.CanDelete(comment.ID, user2.UID)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if canDelete {
		t.Error("Non-owner should not be able to delete")
	}
}

func TestCommentRepository_CanDelete_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewCommentRepository(testDB)
	_, err := repo.CanDelete(999999, 1)
	if !IsRecordNotFound(err) {
		t.Errorf("Expected ErrRecordNotFound, got %v", err)
	}
}
