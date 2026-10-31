package repository

import (
	"errors"

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

	if err := repo.Create(testCtx, token); err != nil {
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
	repo.Create(testCtx, token)

	found, err := repo.FindByHash(testCtx, "findhash")
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
	_, err := repo.FindByHash(testCtx, "nonexistent")
	if !errors.Is(err, model.ErrLoginExpired) {
		t.Errorf("令牌缺失或已过期应回 model.ErrLoginExpired，got %v", err)
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
	repo.Create(testCtx, token)

	_, err := repo.FindByHash(testCtx, "expiredhash")
	if !errors.Is(err, model.ErrLoginExpired) {
		t.Errorf("过期令牌应回 model.ErrLoginExpired, got %v", err)
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
	repo.Create(testCtx, token)

	deleted, err := repo.DeleteByID(testCtx, token.ID)
	if err != nil {
		t.Fatalf("Failed to delete token: %v", err)
	}
	if !deleted {
		t.Fatal("first DeleteByID should consume the token")
	}

	// 第二次删除必须报告 0 行：并发刷新时后来者据此拒绝签发新令牌。
	deleted, err = repo.DeleteByID(testCtx, token.ID)
	if err != nil {
		t.Fatalf("second DeleteByID: %v", err)
	}
	if deleted {
		t.Fatal("second DeleteByID must not report a consumed token")
	}

	_, err = repo.FindByHash(testCtx, "deletehash")
	if !errors.Is(err, model.ErrLoginExpired) {
		t.Errorf("删除后令牌应回 model.ErrLoginExpired, got %v", err)
	}
}

func TestRefreshTokenRepository_DeleteByUserID(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	// 为用户1创建多个 token
	for i := 0; i < 3; i++ {
		repo.Create(testCtx, &model.RefreshToken{
			TokenHash: "hash_user1_" + string(rune('a'+i)),
			UserID:    1,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})
	}
	// 为用户2创建 token
	repo.Create(testCtx, &model.RefreshToken{
		TokenHash: "hash_user2",
		UserID:    2,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	if err := repo.DeleteByUserID(testCtx, 1); err != nil {
		t.Fatalf("Failed to delete tokens: %v", err)
	}

	// 用户1的 token 应全部删除
	_, err := repo.FindByHash(testCtx, "hash_user1_a")
	if !errors.Is(err, model.ErrLoginExpired) {
		t.Errorf("Expected user1 token deleted, got %v", err)
	}

	// 用户2的 token 应保留
	found, err := repo.FindByHash(testCtx, "hash_user2")
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
	repo.Create(testCtx, &model.RefreshToken{
		TokenHash: "expired1",
		UserID:    1,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})
	repo.Create(testCtx, &model.RefreshToken{
		TokenHash: "expired2",
		UserID:    2,
		ExpiresAt: time.Now().Add(-24 * time.Hour),
	})
	// 有效 token
	repo.Create(testCtx, &model.RefreshToken{
		TokenHash: "valid1",
		UserID:    1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	if err := repo.DeleteExpired(testCtx); err != nil {
		t.Fatalf("Failed to delete expired tokens: %v", err)
	}

	// 过期的应被删除
	_, err := repo.FindByHash(testCtx, "expired1")
	if !errors.Is(err, model.ErrLoginExpired) {
		t.Errorf("Expected expired1 deleted, got %v", err)
	}

	// 有效的应保留
	found, err := repo.FindByHash(testCtx, "valid1")
	if err != nil {
		t.Fatalf("Valid token should still exist: %v", err)
	}
	if found.TokenHash != "valid1" {
		t.Errorf("Expected hash 'valid1', got '%s'", found.TokenHash)
	}
}
