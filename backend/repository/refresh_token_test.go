package repository

import (
	"ebook-server/model"
	"testing"
	"time"
)

func TestRefreshTokenRepository_Create(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	token := &model.RefreshToken{
		TokenHash: "abc123hash",
		UserID:    1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	if err := repo.Create(token); err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}
	if token.ID == 0 {
		t.Error("Expected non-zero ID after creation")
	}
}

func TestRefreshTokenRepository_FindByHash_Found(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	token := &model.RefreshToken{
		TokenHash: "findhash",
		UserID:    1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	repo.Create(token)

	found, err := repo.FindByHash("findhash")
	if err != nil {
		t.Fatalf("Failed to find token: %v", err)
	}
	if found.TokenHash != "findhash" {
		t.Errorf("Expected hash 'findhash', got '%s'", found.TokenHash)
	}
}

func TestRefreshTokenRepository_FindByHash_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	_, err := repo.FindByHash("nonexistent")
	if !IsRecordNotFound(err) {
		t.Errorf("Expected ErrRecordNotFound, got %v", err)
	}
}

func TestRefreshTokenRepository_FindByHash_Expired(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	token := &model.RefreshToken{
		TokenHash: "expiredhash",
		UserID:    1,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // 已过期
	}
	repo.Create(token)

	_, err := repo.FindByHash("expiredhash")
	if !IsRecordNotFound(err) {
		t.Errorf("Expected ErrRecordNotFound for expired token, got %v", err)
	}
}

func TestRefreshTokenRepository_DeleteByID(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	token := &model.RefreshToken{
		TokenHash: "deletehash",
		UserID:    1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	repo.Create(token)

	if err := repo.DeleteByID(token.ID); err != nil {
		t.Fatalf("Failed to delete token: %v", err)
	}

	_, err := repo.FindByHash("deletehash")
	if !IsRecordNotFound(err) {
		t.Errorf("Expected ErrRecordNotFound after deletion, got %v", err)
	}
}

func TestRefreshTokenRepository_DeleteByUserID(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	// 为用户1创建多个 token
	for i := 0; i < 3; i++ {
		repo.Create(&model.RefreshToken{
			TokenHash: "hash_user1_" + string(rune('a'+i)),
			UserID:    1,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})
	}
	// 为用户2创建 token
	repo.Create(&model.RefreshToken{
		TokenHash: "hash_user2",
		UserID:    2,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	if err := repo.DeleteByUserID(1); err != nil {
		t.Fatalf("Failed to delete tokens: %v", err)
	}

	// 用户1的 token 应全部删除
	_, err := repo.FindByHash("hash_user1_a")
	if !IsRecordNotFound(err) {
		t.Errorf("Expected user1 token deleted, got %v", err)
	}

	// 用户2的 token 应保留
	found, err := repo.FindByHash("hash_user2")
	if err != nil {
		t.Fatalf("User2 token should still exist: %v", err)
	}
	if found.UserID != 2 {
		t.Errorf("Expected UserID 2, got %d", found.UserID)
	}
}

func TestRefreshTokenRepository_DeleteExpired(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	// 过期 token
	repo.Create(&model.RefreshToken{
		TokenHash: "expired1",
		UserID:    1,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})
	repo.Create(&model.RefreshToken{
		TokenHash: "expired2",
		UserID:    2,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
	})
	// 有效 token
	repo.Create(&model.RefreshToken{
		TokenHash: "valid1",
		UserID:    1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	if err := repo.DeleteExpired(); err != nil {
		t.Fatalf("Failed to delete expired tokens: %v", err)
	}

	// 过期的应被删除
	_, err := repo.FindByHash("expired1")
	if !IsRecordNotFound(err) {
		t.Errorf("Expected expired1 deleted, got %v", err)
	}

	// 有效的应保留
	found, err := repo.FindByHash("valid1")
	if err != nil {
		t.Fatalf("Valid token should still exist: %v", err)
	}
	if found.TokenHash != "valid1" {
		t.Errorf("Expected hash 'valid1', got '%s'", found.TokenHash)
	}
}
