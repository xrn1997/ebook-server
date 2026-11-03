package service

import (
	"context"
	"testing"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/testdb"
	"ebook-server/repository"

	"gorm.io/gorm"
)

// 会话模块的用例自成一套依赖（独立 :memory: 库）：只覆盖 Sessions 的公共路径，
// 不需要验证码/access token 单元，也不占用 test_helper_test.go 的包级夹具。

// newSessionsForTest 组装被测 Sessions 与它背后的三样东西：两间仓库 + 底层连接
// （连接只给需要直接查表的用例，比如数库里到底留了几行令牌）。
func newSessionsForTest(t *testing.T) (*Sessions, *repository.UserRepository, *repository.RefreshTokenRepository, *gorm.DB) {
	t.Helper()
	db := testdb.Open(t)
	users := repository.NewUserRepository(db)
	tokens := repository.NewRefreshTokenRepository(db)
	return NewSessions(tokens, users), users, tokens, db
}

// seedSessionUser 直接经数据层造一个可用账号：口令哈希非空即「未匿名化」。
func seedSessionUser(t *testing.T, users *repository.UserRepository, email string) *model.User {
	t.Helper()
	user := &model.User{
		Email:    email,
		Username: "sess_" + email,
		Nickname: "sess",
		Password: "$2a$10$HS7moy/oOUiT4C7sNcetju2f/0OFL9f3nPGbVBo6cufC1pwP7XNou",
	}
	if err := users.Create(testCtx, user); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return user
}

func TestSessions_IssueThenConsume_RotatesInPlace(t *testing.T) {
	sessions, users, tokens, db := newSessionsForTest(t)
	user := seedSessionUser(t, users, "sess-issue@example.com")

	raw, err := sessions.Issue(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(raw) != 64 {
		t.Fatalf("raw token = %q, want 64 位 hex（32 字节随机）", raw)
	}

	// 只有哈希落库：拿原文去查必须查不到
	if _, err := tokens.FindByHash(testCtx, raw); err != model.ErrLoginExpired {
		t.Fatalf("原始串不得明文落库, FindByHash(raw) err = %v", err)
	}
	record, err := tokens.FindByHash(testCtx, sha256Hex(raw))
	if err != nil {
		t.Fatalf("签发后应能按哈希查到令牌: %v", err)
	}
	if record.UserID != user.UID {
		t.Fatalf("落库记录 user_id = %d, want %d", record.UserID, user.UID)
	}
	if !record.ExpiresAt.After(time.Now().Add(RefreshTokenTTL - time.Hour)) {
		t.Fatalf("落库记录的到期时间不是 now+RefreshTokenTTL: %v", record.ExpiresAt)
	}

	holder, rotated, err := sessions.Consume(testCtx, raw)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if holder.UID != user.UID {
		t.Fatalf("Consume 返回持有者 uid = %d, want %d", holder.UID, user.UID)
	}
	if rotated == raw || len(rotated) != 64 {
		t.Fatalf("轮换必须给出新的一枚原文, got %q", rotated)
	}

	// 就地轮换：旧原文不再成一行，新原文指向同一行、同一账号、同样 30 天。
	if _, err := tokens.FindByHash(testCtx, sha256Hex(raw)); err != model.ErrLoginExpired {
		t.Fatalf("旧令牌应随轮换失效, got %v", err)
	}
	after, err := tokens.FindByHash(testCtx, sha256Hex(rotated))
	if err != nil {
		t.Fatalf("新令牌应按哈希查到: %v", err)
	}
	if after.ID != record.ID || after.UserID != user.UID {
		t.Fatalf("轮换应在同一行上: got id=%d user_id=%d, want id=%d user_id=%d", after.ID, after.UserID, record.ID, user.UID)
	}
	if !after.ExpiresAt.After(time.Now().Add(RefreshTokenTTL - time.Hour)) {
		t.Fatalf("新令牌的到期时间不是 now+RefreshTokenTTL: %v", after.ExpiresAt)
	}
	// 库里始终只有一行：轮换不留第二行——旧写法「删旧行 + 插新行」会把新行落在
	// 并发撤销之后，等于给已注销账号留一枚 30 天有效期的活口。
	var rows int64
	if err := db.Model(&model.RefreshToken{}).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("轮换后该库有 %d 行令牌, want 1", rows)
	}

	// 单次消费：同一枚令牌第二次兑换必须被拒
	if _, _, err := sessions.Consume(testCtx, raw); err != model.ErrLoginExpired {
		t.Fatalf("第二次 Consume 应回 ErrLoginExpired, got %v", err)
	}
}

// raceLoserStore 复现轮换竞争的最坏交错：本请求 FindByHash 之后、轮换落库之前，
// 并发的胜者把同一行换掉了。形态沿用 account_test.go 的 rotationRaceStore——
// 在真实仓库上插一段钩子，而不是手写一个永远返回 0 行的假仓库。
type raceLoserStore struct {
	refreshTokenStore
	onFound func()
}

func (s *raceLoserStore) FindByHash(ctx context.Context, hash string) (*model.RefreshToken, error) {
	record, err := s.refreshTokenStore.FindByHash(ctx, hash)
	if err == nil && s.onFound != nil {
		hook := s.onFound
		s.onFound = nil // 只插一次
		hook()
	}
	return record, err
}

// TestSessions_Consume_RaceLosesWithErrLoginExpired 并发刷新的败者被当作「令牌已失效」拒绝。
func TestSessions_Consume_RaceLosesWithErrLoginExpired(t *testing.T) {
	db := testdb.Open(t)
	users := repository.NewUserRepository(db)
	tokensRepo := repository.NewRefreshTokenRepository(db)
	user := seedSessionUser(t, users, "sess-race@example.com")

	store := &raceLoserStore{refreshTokenStore: tokensRepo}
	sessions := NewSessions(store, users)

	raw, err := sessions.Issue(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// 两方都已查到同一行之后，并发的胜者先完成轮换
	store.onFound = func() {
		record, err := tokensRepo.FindByHash(testCtx, sha256Hex(raw))
		if err != nil {
			t.Fatalf("胜者查令牌: %v", err)
		}
		if rotated, err := tokensRepo.Rotate(testCtx, record.ID, sha256Hex(raw), "winner-hash", time.Now().Add(RefreshTokenTTL)); err != nil || !rotated {
			t.Fatalf("胜者轮换令牌: rotated=%v err=%v", rotated, err)
		}
	}

	if _, _, err := sessions.Consume(testCtx, raw); err != model.ErrLoginExpired {
		t.Fatalf("竞争败者（CAS 打不中）应回 ErrLoginExpired, got %v", err)
	}
}

// TestSessions_Consume_RejectsRotationAfterRevoke 撤销落在「查到旧行」与「轮换落库」之间时，
// 本次刷新必须失败关闭，而不是把新令牌写在撤销之后。
//
// 这是登出/改密/注销与并发刷新的最坏交错：旧写法（删旧行 + 插新行）在这里会让新令牌
// 躲过按 user_id 的批量撤销，活满整个 TTL；改成打在旧哈希上的条件更新后，行已不在，
// CAS 得到 0 行，会话不可能被复活。
func TestSessions_Consume_RejectsRotationAfterRevoke(t *testing.T) {
	db := testdb.Open(t)
	users := repository.NewUserRepository(db)
	tokensRepo := repository.NewRefreshTokenRepository(db)
	user := seedSessionUser(t, users, "sess-revoke-race@example.com")

	store := &raceLoserStore{refreshTokenStore: tokensRepo}
	sessions := NewSessions(store, users)

	raw, err := sessions.Issue(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	store.onFound = func() {
		if err := sessions.Revoke(testCtx, user.UID); err != nil {
			t.Fatalf("胜者撤销: %v", err)
		}
	}

	if _, _, err := sessions.Consume(testCtx, raw); err != model.ErrLoginExpired {
		t.Fatalf("撤销在先的轮换应失败关闭并回 ErrLoginExpired, got %v", err)
	}
	var rows int64
	if err := db.Model(&model.RefreshToken{}).Where("user_id = ?", user.UID).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("撤销后残留 %d 行令牌, want 0（轮换复活了会话）", rows)
	}
}

func TestSessions_Consume_RejectsAnonymizedUser(t *testing.T) {
	sessions, users, tokens, _ := newSessionsForTest(t)
	user := seedSessionUser(t, users, "sess-anon@example.com")

	raw, err := sessions.Issue(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// 匿名化（唯一写入方是 anonymizeUser；空口令哈希即「已注销」）
	anonymizeUser(user)
	if err := users.Update(testCtx, user); err != nil {
		t.Fatalf("anonymize: %v", err)
	}

	if _, _, err := sessions.Consume(testCtx, raw); err != model.ErrAccountNotFound {
		t.Fatalf("匿名化账号的令牌应回 ErrAccountNotFound, got %v", err)
	}

	// 检查发生在轮换之前：这枚令牌行留在库里，但它永远续不了期
	if _, err := tokens.FindByHash(testCtx, sha256Hex(raw)); err != nil {
		t.Fatalf("拒绝匿名化时不消费令牌行, err = %v", err)
	}
}

func TestSessions_Revoke_RemovesAllForUser(t *testing.T) {
	sessions, users, _, _ := newSessionsForTest(t)
	mine := seedSessionUser(t, users, "sess-revoke@example.com")
	other := seedSessionUser(t, users, "sess-revoke-other@example.com")

	first, err := sessions.Issue(testCtx, mine.UID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	second, err := sessions.Issue(testCtx, mine.UID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	untouched, err := sessions.Issue(testCtx, other.UID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if err := sessions.Revoke(testCtx, mine.UID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	for i, raw := range []string{first, second} {
		if _, _, err := sessions.Consume(testCtx, raw); err != model.ErrLoginExpired {
			t.Fatalf("被撤销的第 %d 枚令牌应回 ErrLoginExpired, got %v", i+1, err)
		}
	}
	holder, _, err := sessions.Consume(testCtx, untouched)
	if err != nil {
		t.Fatalf("撤销不得误伤其他账号: %v", err)
	}
	if holder.UID != other.UID {
		t.Fatalf("其他账号会话的持有者 = %d, want %d", holder.UID, other.UID)
	}
}
