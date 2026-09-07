package repository

import (
	"testing"
	"time"

	"ebook-server/model"
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

func TestCommentRepository_FindByChapterURLs_Union(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{Email: "multi@example.com", Password: "hp", Username: "multi", Nickname: "multi"}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: user.UID, Content: "A1", ChapterURL: "key-a", BookName: "书A"})
	repo.Create(&model.Comment{UserID: user.UID, Content: "A2", ChapterURL: "key-a", BookName: "书A"})
	repo.Create(&model.Comment{UserID: user.UID, Content: "B1", ChapterURL: "key-b", BookName: "书A"})
	repo.Create(&model.Comment{UserID: user.UID, Content: "C1", ChapterURL: "key-c", BookName: "书B"})

	// 多键并集：key-a + key-b = 3 条
	comments, total, err := repo.FindByChapterURLs([]string{"key-a", "key-b"}, "", 1, 10)
	if err != nil {
		t.Fatalf("FindByChapterURLs failed: %v", err)
	}
	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
	if len(comments) != 3 {
		t.Errorf("Expected 3 comments, got %d", len(comments))
	}
}

func TestCommentRepository_FindByChapterURLs_WithBookName(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{Email: "bn@example.com", Password: "hp", Username: "bn", Nickname: "bn"}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: user.UID, Content: "A1", ChapterURL: "key-a", BookName: "书A"})
	repo.Create(&model.Comment{UserID: user.UID, Content: "B1", ChapterURL: "key-b", BookName: "书B"})

	// bookName 二次过滤：只返回书A的
	comments, total, err := repo.FindByChapterURLs([]string{"key-a", "key-b"}, "书A", 1, 10)
	if err != nil {
		t.Fatalf("FindByChapterURLs with bookName failed: %v", err)
	}
	if total != 1 {
		t.Errorf("Expected total 1, got %d", total)
	}
	if len(comments) != 1 || comments[0].Content != "A1" {
		t.Errorf("Expected [A1], got %v", comments)
	}
}

func TestCommentRepository_FindByChapterURLs_Pagination(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{Email: "pg@example.com", Password: "hp", Username: "pg", Nickname: "pg"}
	userRepo.Create(user)

	repo := NewCommentRepository(testDB)
	for i := 0; i < 5; i++ {
		repo.Create(&model.Comment{UserID: user.UID, Content: "A", ChapterURL: "key-a"})
	}
	for i := 0; i < 3; i++ {
		repo.Create(&model.Comment{UserID: user.UID, Content: "B", ChapterURL: "key-b"})
	}

	// 8 条并集，分页 pageSize=3
	comments, total, err := repo.FindByChapterURLs([]string{"key-a", "key-b"}, "", 1, 3)
	if err != nil {
		t.Fatalf("FindByChapterURLs pagination failed: %v", err)
	}
	if total != 8 {
		t.Errorf("Expected total 8, got %d", total)
	}
	if len(comments) != 3 {
		t.Errorf("Expected 3 comments on page 1, got %d", len(comments))
	}
}

func TestCommentRepository_FindByChapterURLs_Empty(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewCommentRepository(testDB)
	comments, total, err := repo.FindByChapterURLs([]string{"nonexistent"}, "", 1, 10)
	if err != nil {
		t.Fatalf("FindByChapterURLs empty failed: %v", err)
	}
	if total != 0 || len(comments) != 0 {
		t.Errorf("Expected empty result, got total=%d len=%d", total, len(comments))
	}
}

// ── M2 主读路径：按 comment_key 过滤 ─────────────────────────────────────

// TestCommentRepository_FindByCommentKeys_SingleKey 单键查询即精确匹配该键。
//
// 聚合键由客户端派生、服务端不解释其形态，所以这里只验证等值匹配与 Preload User。
func TestCommentRepository_FindByCommentKeys_SingleKey(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "cksingle@example.com", "cksingle", "cksingle")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "A1", CommentKey: "ck1:a#1"})
	repo.Create(&model.Comment{UserID: uid, Content: "A2", CommentKey: "ck1:a#1"})
	repo.Create(&model.Comment{UserID: uid, Content: "B1", CommentKey: "ck1:a#2"})

	comments, total, err := repo.FindByCommentKeys([]string{"ck1:a#1"}, 1, 10)
	if err != nil {
		t.Fatalf("FindByCommentKeys failed: %v", err)
	}
	if total != 2 || len(comments) != 2 {
		t.Fatalf("FindByCommentKeys([ck1:a#1]) total=%d len=%d, want 2/2", total, len(comments))
	}
	for _, c := range comments {
		if c.CommentKey != "ck1:a#1" {
			t.Errorf("matched row comment_key = %q, want ck1:a#1", c.CommentKey)
		}
	}
	// 视图转换依赖关联用户，仓库层就得把 User 预加载出来
	if comments[0].User.UID != uid {
		t.Errorf("preloaded user UID = %d, want %d", comments[0].User.UID, uid)
	}
}

// TestCommentRepository_FindByCommentKeys_Union 多键返回并集，且按 created_at DESC 排序。
//
// 跨书源合并同一作品时客户端会把该作品的多个键一并传来，并集里不能漏行；
// 排序必须按时间而不是按键分组，否则合并后的列表是乱序的。
func TestCommentRepository_FindByCommentKeys_Union(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "ckunion@example.com", "ckunion", "ckunion")
	repo := NewCommentRepository(testDB)

	// 显式指定 created_at：同批插入的时间差太小，排序断言会不稳；
	// 拉开间隔才能证明顺序来自时间而不是插入次序。
	base := time.Now().Add(-time.Hour)
	seed := func(content, key string, age time.Duration) {
		if err := repo.Create(&model.Comment{
			UserID: uid, Content: content, CommentKey: key, CreatedAt: base.Add(age),
		}); err != nil {
			t.Fatalf("seed comment failed: %v", err)
		}
	}
	seed("old-a", "ck1:a", 0)
	seed("mid-b", "ck1:b", time.Minute)
	seed("new-a", "ck1:a", 2*time.Minute)
	seed("other-c", "ck1:c", 3*time.Minute) // 不在过滤集合内

	comments, total, err := repo.FindByCommentKeys([]string{"ck1:a", "ck1:b"}, 1, 10)
	if err != nil {
		t.Fatalf("FindByCommentKeys union failed: %v", err)
	}
	if total != 3 || len(comments) != 3 {
		t.Fatalf("union total=%d len=%d, want 3/3", total, len(comments))
	}
	want := []string{"new-a", "mid-b", "old-a"}
	for i, content := range want {
		if comments[i].Content != content {
			t.Errorf("union order[%d] = %q, want %q (created_at DESC)", i, comments[i].Content, content)
		}
	}
}

// TestCommentRepository_FindByCommentKeys_Pagination 并集分页：total 是全量，items 只取当页。
func TestCommentRepository_FindByCommentKeys_Pagination(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "ckpg@example.com", "ckpg", "ckpg")
	repo := NewCommentRepository(testDB)
	for i := 0; i < 5; i++ {
		repo.Create(&model.Comment{UserID: uid, Content: "A", CommentKey: "ck1:a"})
	}
	for i := 0; i < 3; i++ {
		repo.Create(&model.Comment{UserID: uid, Content: "B", CommentKey: "ck1:b"})
	}

	keys := []string{"ck1:a", "ck1:b"}
	comments, total, err := repo.FindByCommentKeys(keys, 1, 3)
	if err != nil {
		t.Fatalf("FindByCommentKeys page 1 failed: %v", err)
	}
	if total != 8 {
		t.Errorf("total = %d, want 8（并集全量，不受分页影响）", total)
	}
	if len(comments) != 3 {
		t.Errorf("page 1 len = %d, want 3", len(comments))
	}

	// 末页只装余下的 2 条
	last, _, _ := repo.FindByCommentKeys(keys, 3, 3)
	if len(last) != 2 {
		t.Errorf("page 3 len = %d, want 2", len(last))
	}

	// 越界页：空列表但 total 仍是全量（客户端据此判断还有多少页）
	empty, total9, _ := repo.FindByCommentKeys(keys, 9, 3)
	if len(empty) != 0 || total9 != 8 {
		t.Errorf("page 9 len=%d total=%d, want 0/8", len(empty), total9)
	}
}

// TestCommentRepository_FindByCommentKeys_SkipsUnkeyedRows comment_key 为空的行不属于任何桶。
//
// 空键 = 尚未换键的历史行（M2 之前入库、服务端再也算不出键）。非空键过滤必须把它们
// 排除在外，否则一次错键迁移就会把无主评论混进别人的章节。
func TestCommentRepository_FindByCommentKeys_SkipsUnkeyedRows(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "ckunkeyed@example.com", "ckunkeyed", "ckunkeyed")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "unkeyed"})
	repo.Create(&model.Comment{UserID: uid, Content: "keyed", CommentKey: "ck1:a"})

	comments, total, err := repo.FindByCommentKeys([]string{"ck1:a"}, 1, 10)
	if err != nil {
		t.Fatalf("FindByCommentKeys failed: %v", err)
	}
	if total != 1 || len(comments) != 1 || comments[0].Content != "keyed" {
		t.Errorf("non-empty key filter total=%d got=%+v, want 1/[keyed]", total, comments)
	}

	// 反向：只有显式按空键过滤才捞得到未换键的行（迁移前统计待收拢数量的用法）。
	// handler 会丢弃空串键，所以这条路径不会出现在公开查询里。
	if _, total, _ := repo.FindByCommentKeys([]string{""}, 1, 10); total != 1 {
		t.Errorf("empty key total = %d, want 1", total)
	}
}

// TestCommentRepository_FindByCommentKeys_NoMatch 不存在的键返回空结果而非错误。
func TestCommentRepository_FindByCommentKeys_NoMatch(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewCommentRepository(testDB)
	comments, total, err := repo.FindByCommentKeys([]string{"ck1:nonexistent"}, 1, 10)
	if err != nil {
		t.Fatalf("FindByCommentKeys failed: %v", err)
	}
	if total != 0 || len(comments) != 0 {
		t.Errorf("Expected empty result, got total=%d len=%d", total, len(comments))
	}
}

// TestCommentRepository_FindByChapterURLs_LegacyRowsReadable 旧读路径仍要能读到未换键的历史行。
//
// M2 前入库的评论只有 chapter_url，服务端无法把它重算成 comment_key（算键需要作者），
// 因此这些行只能靠 chapter_url 继续可见——这是保留整条废弃路径的唯一理由。
func TestCommentRepository_FindByChapterURLs_LegacyRowsReadable(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "legacyurl@example.com", "legacyurl", "legacyurl")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "old-1", ChapterURL: "https://src.example.com/b/1.html"})
	repo.Create(&model.Comment{UserID: uid, Content: "old-2", ChapterURL: "https://src.example.com/b/1.html",
		CommentKey: "ck1:a#1"})

	if _, total, _ := repo.FindByChapterURLs([]string{"https://src.example.com/b/1.html"}, "", 1, 10); total != 2 {
		t.Errorf("legacy chapter_url total = %d, want 2", total)
	}
	// 旧键值不是新键：拿同一个字符串去查 comment_key 必须一无所获，
	// 否则两列的语义就混在一起了。
	if _, total, _ := repo.FindByCommentKeys([]string{"https://src.example.com/b/1.html"}, 1, 10); total != 0 {
		t.Errorf("comment_key filter matched chapter_url values, total = %d, want 0", total)
	}
}

// TestCommentRepository_MigrateKey_Success 迁移改的是 comment_key，chapter_url 不受影响。
//
// 换轨前聚合键就是 chapter_url，所以这里特意留一条「chapter_url = old-key 但
// comment_key = other-key」的行：它必须原地不动，证明旧列不会再被迁移误伤。
func TestCommentRepository_MigrateKey_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "mig@example.com", "mig", "mig")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "c1", CommentKey: "old-key"})
	repo.Create(&model.Comment{UserID: uid, Content: "c2", CommentKey: "old-key"})
	repo.Create(&model.Comment{UserID: uid, Content: "c3", CommentKey: "other-key"})
	repo.Create(&model.Comment{UserID: uid, Content: "legacy", CommentKey: "other-key", ChapterURL: "old-key"})

	count, err := repo.MigrateKey(uid, "old-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 migrated, got %d", count)
	}

	// 迁移结果：两条落进新键
	if _, total, _ := repo.FindByCommentKeys([]string{"new-key"}, 1, 10); total != 2 {
		t.Errorf("comments under new-key = %d, want 2", total)
	}
	// 旧键下应无评论
	if _, total, _ := repo.FindByCommentKeys([]string{"old-key"}, 1, 10); total != 0 {
		t.Errorf("comments under old-key = %d, want 0", total)
	}
	// 其他键不受影响
	if _, total, _ := repo.FindByCommentKeys([]string{"other-key"}, 1, 10); total != 2 {
		t.Errorf("comments under other-key = %d, want 2", total)
	}
	// 幂等重试：再跑一次没有匹配行，返回 0 而不是报错
	if retry, err := repo.MigrateKey(uid, "old-key", "new-key"); err != nil || retry != 0 {
		t.Errorf("retry MigrateKey = (%d, %v), want (0, nil)", retry, err)
	}

	// 废弃列不参与迁移：legacy 的 chapter_url 仍是 old-key，照旧可读
	legacy, legacyTotal, _ := repo.FindByChapterURLs([]string{"old-key"}, "", 1, 10)
	if legacyTotal != 1 || len(legacy) != 1 || legacy[0].CommentKey != "other-key" {
		t.Errorf("chapter_url should be left untouched, got %+v", legacy)
	}
}

// TestCommentRepository_MigrateKey_NoMatch 不匹配的旧键返回 0，且不会顺手收走无键行。
//
// 空串也是 comment_key 的合法取值（未换键的历史行），所以「键写错了」绝不能等于
// 「迁移本人所有无键评论」。
func TestCommentRepository_MigrateKey_NoMatch(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "nomig@example.com", "nomig", "nomig")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "c1", CommentKey: "other-key"})
	repo.Create(&model.Comment{UserID: uid, Content: "unkeyed"})

	count, err := repo.MigrateKey(uid, "nonexistent", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey no-match failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 migrated, got %d", count)
	}
	if _, total, _ := repo.FindByCommentKeys([]string{"new-key"}, 1, 10); total != 0 {
		t.Errorf("no-match migration should create nothing, got %d under new-key", total)
	}
}

// TestCommentRepository_MigrateKey_UserIsolation 迁移只作用于本人，空键收拢也一样。
func TestCommentRepository_MigrateKey_UserIsolation(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user1 := &model.User{Email: "u1@example.com", Password: "hp", Username: "u1", Nickname: "u1"}
	user2 := &model.User{Email: "u2@example.com", Password: "hp", Username: "u2", Nickname: "u2"}
	userRepo.Create(user1)
	userRepo.Create(user2)

	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: user1.UID, Content: "u1c1", CommentKey: "shared-key"})
	repo.Create(&model.Comment{UserID: user1.UID, Content: "u1-unkeyed"})
	repo.Create(&model.Comment{UserID: user2.UID, Content: "u2c1", CommentKey: "shared-key"})
	repo.Create(&model.Comment{UserID: user2.UID, Content: "u2-unkeyed"})

	// 只迁移 user1 的
	count, err := repo.MigrateKey(user1.UID, "shared-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey user isolation failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 migrated, got %d", count)
	}

	// user2 的评论仍在旧键下
	comments, _, _ := repo.FindByCommentKeys([]string{"shared-key"}, 1, 10)
	if len(comments) != 1 || comments[0].UserID != user2.UID {
		t.Errorf("user2's comment should remain under shared-key, got %+v", comments)
	}

	// 空键收拢同样受 user_id 约束：只带走 user1 自己那条未换键的行
	if swept, err := repo.MigrateKey(user1.UID, "", "ch-1"); err != nil || swept != 1 {
		t.Errorf("empty-old-key sweep = (%d, %v), want (1, nil)", swept, err)
	}
	if _, total, _ := repo.FindByCommentKeys([]string{""}, 1, 10); total != 1 {
		t.Errorf("user2's unkeyed row must stay unkeyed, empty-key total = %d, want 1", total)
	}
}

// TestCommentRepository_Search 内容关键字 + 书名筛选，零值条件不过滤。
func TestCommentRepository_Search(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "cs@example.com", "cs", "cs")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "这一章很精彩", BookName: "书A"})
	repo.Create(&model.Comment{UserID: uid, Content: "排版有问题", BookName: "书A"})
	repo.Create(&model.Comment{UserID: uid, Content: "很精彩的序言", BookName: "书B"})

	cases := []struct {
		name  string
		query model.CommentQuery
		want  int64
	}{
		{"关键字", model.CommentQuery{Keyword: "精彩"}, 2},
		{"书名", model.CommentQuery{BookName: "书A"}, 2},
		{"关键字+书名", model.CommentQuery{Keyword: "精彩", BookName: "书A"}, 1},
		{"空调=全量", model.CommentQuery{}, 3},
		{"通配符不被当模式", model.CommentQuery{Keyword: "%"}, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, total, err := repo.Search(c.query, 1, 10)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			if total != c.want {
				t.Errorf("Search(%+v) total = %d, want %d", c.query, total, c.want)
			}
		})
	}
}

// TestCommentRepository_Count 只数未删除评论（后台概览用真实计数）。
func TestCommentRepository_Count(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "cnt@example.com", "cnt", "cnt")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "keep"})
	repo.Create(&model.Comment{UserID: uid, Content: "drop"})
	repo.Delete(2)

	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Count = %d, want 1 (软删除不得计入)", count)
	}

	perUser, err := repo.CountByUserID(uid)
	if err != nil {
		t.Fatalf("CountByUserID failed: %v", err)
	}
	if perUser != 1 {
		t.Errorf("CountByUserID = %d, want 1", perUser)
	}
}

// TestCommentRepository_MigrateKey_BookLevel 旧键为空 = 收拢本人尚未换键的历史行。
//
// 换轨后空串不再是「书籍级评论」而是「还没有 comment_key 的行」——chapter_url 时代的
// 书籍级评论正好落在这批里（那时它们就没有章节 URL）。客户端合并书籍后要做的第一件事
// 就是把它们一次性收进真实的桶，因此 old_key="" 必须精确命中这批行、且只命中这批行。
func TestCommentRepository_MigrateKey_BookLevel(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "migbl@example.com", "migbl", "migbl")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "book-1", CommentKey: ""})
	repo.Create(&model.Comment{UserID: uid, Content: "book-2", CommentKey: ""})
	// 已有键的行（哪怕带着旧 chapter_url）不能被空键迁移碰
	repo.Create(&model.Comment{UserID: uid, Content: "chapter-1", CommentKey: "key-a",
		ChapterURL: "https://src.example.com/b/1.html"})

	migrated, err := repo.MigrateKey(uid, "", "ch-1")
	if err != nil {
		t.Fatalf("MigrateKey failed: %v", err)
	}
	if migrated != 2 {
		t.Errorf("migrated = %d, want 2", migrated)
	}

	if _, total, _ := repo.FindByCommentKeys([]string{"ch-1"}, 1, 10); total != 2 {
		t.Errorf("comments under ch-1 = %d, want 2", total)
	}
	if _, total, _ := repo.FindByCommentKeys([]string{"key-a"}, 1, 10); total != 1 {
		t.Errorf("unrelated key should be untouched, got %d", total)
	}
	// 收拢后不再有待换键的行
	if _, total, _ := repo.FindByCommentKeys([]string{""}, 1, 10); total != 0 {
		t.Errorf("unkeyed rows left = %d, want 0", total)
	}
	// 迁移不改写废弃列：那条行仍可按旧 chapter_url 读到
	if _, total, _ := repo.FindByChapterURLs([]string{"https://src.example.com/b/1.html"}, "", 1, 10); total != 1 {
		t.Errorf("chapter_url snapshot should stay intact, got %d", total)
	}
}

// TestCommentRepository_RehashKey_CrossUser 全局改键不限用户，且只动指定的那一桶。
//
// 这是 RehashKey 与 MigrateKey 在数据层的唯一区别：没有 user_id 过滤。
// 公开端点的 migrate 只搬本人行，错桶上聚着他人评论时救不了场，
// 因此需要一个跨用户的改键原语——它只由后台端点调用（ADR-0010）。
func TestCommentRepository_RehashKey_CrossUser(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	u1 := seedUser(t, "rehash_u1@example.com", "rehash1", "rehash1")
	u2 := seedUser(t, "rehash_u2@example.com", "rehash2", "rehash2")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: u1, Content: "polluted-1", CommentKey: "ck1:polluted"})
	repo.Create(&model.Comment{UserID: u2, Content: "polluted-2", CommentKey: "ck1:polluted"})
	repo.Create(&model.Comment{UserID: u1, Content: "elsewhere", CommentKey: "ck1:other"})
	// 废弃列同名的行不该被牵连：改键只认 comment_key 列
	repo.Create(&model.Comment{UserID: u1, Content: "legacy", ChapterURL: "ck1:polluted"})

	count, err := repo.RehashKey("ck1:polluted", "ck1:fixed")
	if err != nil {
		t.Fatalf("RehashKey failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 rows rehashed across both users, got %d", count)
	}
	if _, total, _ := repo.FindByCommentKeys([]string{"ck1:fixed"}, 1, 10); total != 2 {
		t.Errorf("Expected 2 rows under new key, got %d", total)
	}
	if _, total, _ := repo.FindByCommentKeys([]string{"ck1:polluted"}, 1, 10); total != 0 {
		t.Errorf("Expected old bucket emptied, got %d rows left", total)
	}
	if _, total, _ := repo.FindByCommentKeys([]string{"ck1:other"}, 1, 10); total != 1 {
		t.Errorf("Unrelated bucket should be untouched, got %d", total)
	}
	if _, total, _ := repo.FindByChapterURLs([]string{"ck1:polluted"}, "", 1, 10); total != 1 {
		t.Errorf("Rehash must not touch the deprecated chapter_url column, got %d", total)
	}
}

// TestCommentRepository_RehashKey_NoMatch 无匹配行返回 0 而非报错（幂等，可重复调用）。
func TestCommentRepository_RehashKey_NoMatch(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewCommentRepository(testDB)
	count, err := repo.RehashKey("ck1:missing", "ck1:target")
	if err != nil {
		t.Fatalf("RehashKey no-match failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rehashed, got %d", count)
	}
}
