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

// TestRefreshTokenRepository_Rotate 轮换是一条打在旧哈希上的条件更新：
// 抢到行的一方改到 1 行，哈希与有效期都换掉；拿旧哈希却打不中（已轮换、已撤销）
// 的一方必须收到 false，否则一次性的凭证就成了可无限续期的后门。
func TestRefreshTokenRepository_Rotate(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	token := &model.RefreshToken{
		TokenHash: "rotatehash",
		UserID:    1,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	repo.Create(testCtx, token)

	// CAS 命中：同一行原地换新，身份与归属不变。
	rotated, err := repo.Rotate(testCtx, token.ID, "rotatehash", "rotatedhash", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if !rotated {
		t.Fatal("持有当前哈希的一方应轮换成功")
	}
	found, err := repo.FindByHash(testCtx, "rotatedhash")
	if err != nil {
		t.Fatalf("新哈希应可查到: %v", err)
	}
	if found.ID != token.ID || found.UserID != token.UserID {
		t.Errorf("轮换应在同一行上: got id=%d user_id=%d", found.ID, found.UserID)
	}

	// 旧哈希重放：行还在，但已不是这个哈希 → 拒绝。
	if rotated, err := repo.Rotate(testCtx, token.ID, "rotatehash", "replayhash", time.Now().Add(24*time.Hour)); err != nil || rotated {
		t.Errorf("旧哈希重放应被拒 (rotated=%v err=%v)", rotated, err)
	}

	// 撤销在先：行已删除，轮换不得复活它。
	if err := repo.DeleteByUserID(testCtx, 1); err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}
	if rotated, err := repo.Rotate(testCtx, token.ID, "rotatedhash", "resurrecthash", time.Now().Add(24*time.Hour)); err != nil || rotated {
		t.Errorf("撤销后的轮换应失败关闭 (rotated=%v err=%v)", rotated, err)
	}
	var rows int64
	if err := testDB.Model(&model.RefreshToken{}).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("撤销后轮换留下了 %d 行令牌, want 0", rows)
	}
}

// TestRefreshTokenRepository_Rotate_ExpiredRow 过期的行换不动。
//
// FindByHash 会把过期的行滤掉，所以轮换的正常路径碰不到它；这条防的是两次调用之间的窗口——
// 查到行之后还要做持有者状态检查与随机数生成，令牌可能正好在窗口里到期。UPDATE 里少了
// `expires_at > now`，这一行就会被换成新的 30 天有效期，过期的凭证于是成了续期后门。
func TestRefreshTokenRepository_Rotate_ExpiredRow(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewRefreshTokenRepository(testDB)
	token := &model.RefreshToken{
		TokenHash: "expiredhash",
		UserID:    1,
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	if err := repo.Create(testCtx, token); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if rotated, err := repo.Rotate(testCtx, token.ID, "expiredhash", "renewedhash", time.Now().Add(24*time.Hour)); err != nil || rotated {
		t.Errorf("过期令牌的轮换应失败关闭 (rotated=%v err=%v)", rotated, err)
	}
	var rows int64
	if err := testDB.Model(&model.RefreshToken{}).Where("token_hash = ?", "renewedhash").Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatal("过期令牌被续期了：新哈希已落库")
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
