package repository

import (
	"errors"
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
	userRepo.Create(testCtx, user)

	repo := NewCommentRepository(testDB)
	comment := &model.Comment{
		UserID:  user.UID,
		Content: "This is a test comment",
	}

	if err := repo.Create(testCtx, comment); err != nil {
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
	userRepo.Create(testCtx, user)

	repo := NewCommentRepository(testDB)
	comment := &model.Comment{
		UserID:  user.UID,
		Content: "Find me",
	}
	repo.Create(testCtx, comment)

	found, err := repo.FindByID(testCtx, comment.ID)
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
	_, err := repo.FindByID(testCtx, 999999)
	if !errors.Is(err, model.ErrCommentNotFound) {
		t.Errorf("评论查不到应回 model.ErrCommentNotFound，got %v", err)
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
	userRepo.Create(testCtx, user)

	repo := NewCommentRepository(testDB)
	for i := 0; i < 5; i++ {
		repo.Create(testCtx, &model.Comment{
			UserID:  user.UID,
			Content: "Comment",
		})
	}

	comments, total, err := repo.FindByUserID(testCtx, user.UID, 1, 3)
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
	userRepo.Create(testCtx, user)

	repo := NewCommentRepository(testDB)
	for i := 0; i < 5; i++ {
		repo.Create(testCtx, &model.Comment{
			UserID:  user.UID,
			Content: "Export comment",
		})
	}

	comments, err := repo.FindAllByUserID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Failed to find comments: %v", err)
	}
	if len(comments) != 5 {
		t.Errorf("Expected 5 comments, got %d", len(comments))
	}
}

// TestCommentRepository_List_NoFilter 空过滤条件走 List 的兜底分支：全局最新一页。
//
// 这里测的是 List（生产唯一的读入口），不是某个 finder——全量列表曾单列过
// 一个只差 WHERE 的 FindAll，现在它是 Search 的零值条件特例。
func TestCommentRepository_List_NoFilter(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "allcomments@example.com",
		Password: "hashedpassword",
		Username: "allcomments",
		Nickname: "allcomments",
	}
	userRepo.Create(testCtx, user)

	repo := NewCommentRepository(testDB)
	for i := 0; i < 15; i++ {
		repo.Create(testCtx, &model.Comment{
			UserID:  user.UID,
			Content: "Comment",
		})
	}

	// 第一页
	comments, total, err := repo.List(testCtx, model.CommentFilter{Page: 1, PageSize: 10})
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
	comments2, _, _ := repo.List(testCtx, model.CommentFilter{Page: 2, PageSize: 10})
	if len(comments2) != 5 {
		t.Errorf("Expected 5 comments on page 2, got %d", len(comments2))
	}
}

// TestCommentRepository_List_UserZeroScopeMatchesNobody uid 为 0 且声明按用户过滤时，
// 必须匹配 0 行，而不是退化成「所有人的评论」。
//
// CommentFilter 把 ByUserID 与 UserID 分成两个字段，就是为了不让 uid == 0 同时充当
// 「不按用户过滤」的哨兵（见 model.CommentFilter 的注释）。这条用例是该设计在读取
// 路径上的唯一真断言：List 的分支一旦退回 `UserID != 0 || ByUserID`，这里就会拿到
// 全部行——而调用方只是没传 uid。
func TestCommentRepository_List_UserZeroScopeMatchesNobody(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "uidzeroscope@example.com",
		Password: "hashedpassword",
		Username: "uidzeroscope",
		Nickname: "uidzeroscope",
	}
	if err := userRepo.Create(testCtx, user); err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}

	repo := NewCommentRepository(testDB)
	for i := 0; i < 3; i++ {
		repo.Create(testCtx, &model.Comment{UserID: user.UID, Content: "Comment"})
	}

	comments, total, err := repo.List(testCtx, model.CommentFilter{ByUserID: true, UserID: 0, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 0 || len(comments) != 0 {
		t.Errorf("按 uid 0 过滤拿到 %d 条（total=%d），应为 0 条：把 0 当「不限用户」的哨兵会把别人的评论递出去",
			len(comments), total)
	}
}

// TestCommentRepository_DeleteScoped_AuthorScope 作者 scope 同时匹配 id 与 user_id：
// 别人的评论删不动，自己的能删。
func TestCommentRepository_DeleteScoped_AuthorScope(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	owner := &model.User{Email: "owner@example.com", Password: "hp", Username: "owner", Nickname: "owner"}
	other := &model.User{Email: "other@example.com", Password: "hp", Username: "other", Nickname: "other"}
	userRepo.Create(testCtx, owner)
	userRepo.Create(testCtx, other)

	repo := NewCommentRepository(testDB)
	comment := &model.Comment{UserID: owner.UID, Content: "Owned comment"}
	repo.Create(testCtx, comment)

	deleted, err := repo.DeleteScoped(testCtx, comment.ID, model.CommentScopeOfAuthor(other.UID))
	if err != nil {
		t.Fatalf("DeleteScoped failed: %v", err)
	}
	if deleted {
		t.Fatal("非作者不应删掉别人的评论")
	}
	if _, err := repo.FindByID(testCtx, comment.ID); err != nil {
		t.Fatalf("评论应仍在: %v", err)
	}

	deleted, err = repo.DeleteScoped(testCtx, comment.ID, model.CommentScopeOfAuthor(owner.UID))
	if err != nil {
		t.Fatalf("DeleteScoped failed: %v", err)
	}
	if !deleted {
		t.Fatal("作者应能删除自己的评论")
	}
	if _, err := repo.FindByID(testCtx, comment.ID); !errors.Is(err, model.ErrCommentNotFound) {
		t.Errorf("软删后查不到应回 model.ErrCommentNotFound, got %v", err)
	}
}

// TestCommentRepository_DeleteScoped_AdminBypassesOwnership 治理删不挂 user_id；
// 重复删除返回 false 而不是假成功。
func TestCommentRepository_DeleteScoped_AdminBypassesOwnership(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	owner := &model.User{Email: "admin-scope@example.com", Password: "hp", Username: "owner", Nickname: "owner"}
	userRepo.Create(testCtx, owner)

	repo := NewCommentRepository(testDB)
	comment := &model.Comment{UserID: owner.UID, Content: "governance"}
	repo.Create(testCtx, comment)

	deleted, err := repo.DeleteScoped(testCtx, comment.ID, model.CommentScopeOfAdmin())
	if err != nil || !deleted {
		t.Fatalf("admin DeleteScoped = %v, %v; want true, nil", deleted, err)
	}
	deleted, err = repo.DeleteScoped(testCtx, comment.ID, model.CommentScopeOfAdmin())
	if err != nil {
		t.Fatalf("second DeleteScoped: %v", err)
	}
	if deleted {
		t.Fatal("重复删除应返回 false（RowsAffected 是唯一依据）")
	}
}

func TestCommentRepository_DeleteScoped_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewCommentRepository(testDB)
	deleted, err := repo.DeleteScoped(testCtx, 999999, model.CommentScopeOfAdmin())
	if err != nil {
		t.Fatalf("DeleteScoped: %v", err)
	}
	if deleted {
		t.Fatal("不存在的评论不应报告删除成功")
	}
}

func TestCommentRepository_FindByChapterURLs_Union(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	userRepo := NewUserRepository(testDB)
	user := &model.User{Email: "multi@example.com", Password: "hp", Username: "multi", Nickname: "multi"}
	userRepo.Create(testCtx, user)

	repo := NewCommentRepository(testDB)
	repo.Create(testCtx, &model.Comment{UserID: user.UID, Content: "A1", ChapterURL: "key-a", BookName: "书A"})
	repo.Create(testCtx, &model.Comment{UserID: user.UID, Content: "A2", ChapterURL: "key-a", BookName: "书A"})
	repo.Create(testCtx, &model.Comment{UserID: user.UID, Content: "B1", ChapterURL: "key-b", BookName: "书A"})
	repo.Create(testCtx, &model.Comment{UserID: user.UID, Content: "C1", ChapterURL: "key-c", BookName: "书B"})

	// 多键并集：key-a + key-b = 3 条
	comments, total, err := repo.FindByChapterURLs(testCtx, []string{"key-a", "key-b"}, "", 1, 10)
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
	userRepo.Create(testCtx, user)

	repo := NewCommentRepository(testDB)
	repo.Create(testCtx, &model.Comment{UserID: user.UID, Content: "A1", ChapterURL: "key-a", BookName: "书A"})
	repo.Create(testCtx, &model.Comment{UserID: user.UID, Content: "B1", ChapterURL: "key-b", BookName: "书B"})

	// bookName 二次过滤：只返回书A的
	comments, total, err := repo.FindByChapterURLs(testCtx, []string{"key-a", "key-b"}, "书A", 1, 10)
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
	userRepo.Create(testCtx, user)

	repo := NewCommentRepository(testDB)
	for i := 0; i < 5; i++ {
		repo.Create(testCtx, &model.Comment{UserID: user.UID, Content: "A", ChapterURL: "key-a"})
	}
	for i := 0; i < 3; i++ {
		repo.Create(testCtx, &model.Comment{UserID: user.UID, Content: "B", ChapterURL: "key-b"})
	}

	// 8 条并集，分页 pageSize=3
	comments, total, err := repo.FindByChapterURLs(testCtx, []string{"key-a", "key-b"}, "", 1, 3)
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
	comments, total, err := repo.FindByChapterURLs(testCtx, []string{"nonexistent"}, "", 1, 10)
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
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "A1", CommentKey: "ck1:a#1"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "A2", CommentKey: "ck1:a#1"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "B1", CommentKey: "ck1:a#2"})

	comments, total, err := repo.FindByCommentKeys(testCtx, []string{"ck1:a#1"}, 1, 10)
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
		if err := repo.Create(testCtx, &model.Comment{
			UserID: uid, Content: content, CommentKey: key, CreatedAt: base.Add(age),
		}); err != nil {
			t.Fatalf("seed comment failed: %v", err)
		}
	}
	seed("old-a", "ck1:a", 0)
	seed("mid-b", "ck1:b", time.Minute)
	seed("new-a", "ck1:a", 2*time.Minute)
	seed("other-c", "ck1:c", 3*time.Minute) // 不在过滤集合内

	comments, total, err := repo.FindByCommentKeys(testCtx, []string{"ck1:a", "ck1:b"}, 1, 10)
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
		repo.Create(testCtx, &model.Comment{UserID: uid, Content: "A", CommentKey: "ck1:a"})
	}
	for i := 0; i < 3; i++ {
		repo.Create(testCtx, &model.Comment{UserID: uid, Content: "B", CommentKey: "ck1:b"})
	}

	keys := []string{"ck1:a", "ck1:b"}
	comments, total, err := repo.FindByCommentKeys(testCtx, keys, 1, 3)
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
	last, _, _ := repo.FindByCommentKeys(testCtx, keys, 3, 3)
	if len(last) != 2 {
		t.Errorf("page 3 len = %d, want 2", len(last))
	}

	// 越界页：空列表但 total 仍是全量（客户端据此判断还有多少页）
	empty, total9, _ := repo.FindByCommentKeys(testCtx, keys, 9, 3)
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
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "unkeyed"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "keyed", CommentKey: "ck1:a"})

	comments, total, err := repo.FindByCommentKeys(testCtx, []string{"ck1:a"}, 1, 10)
	if err != nil {
		t.Fatalf("FindByCommentKeys failed: %v", err)
	}
	if total != 1 || len(comments) != 1 || comments[0].Content != "keyed" {
		t.Errorf("non-empty key filter total=%d got=%+v, want 1/[keyed]", total, comments)
	}

	// 反向：只有显式按空键过滤才捞得到未换键的行（迁移前统计待收拢数量的用法）。
	// handler 会丢弃空串键，所以这条路径不会出现在公开查询里。
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{""}, 1, 10); total != 1 {
		t.Errorf("empty key total = %d, want 1", total)
	}
}

// TestCommentRepository_FindByCommentKeys_NoMatch 不存在的键返回空结果而非错误。
func TestCommentRepository_FindByCommentKeys_NoMatch(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewCommentRepository(testDB)
	comments, total, err := repo.FindByCommentKeys(testCtx, []string{"ck1:nonexistent"}, 1, 10)
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
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "old-1", ChapterURL: "https://src.example.com/b/1.html"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "old-2", ChapterURL: "https://src.example.com/b/1.html",
		CommentKey: "ck1:a#1"})

	if _, total, _ := repo.FindByChapterURLs(testCtx, []string{"https://src.example.com/b/1.html"}, "", 1, 10); total != 2 {
		t.Errorf("legacy chapter_url total = %d, want 2", total)
	}
	// 旧键值不是新键：拿同一个字符串去查 comment_key 必须一无所获，
	// 否则两列的语义就混在一起了。
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"https://src.example.com/b/1.html"}, 1, 10); total != 0 {
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
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "c1", CommentKey: "old-key"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "c2", CommentKey: "old-key"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "c3", CommentKey: "other-key"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "legacy", CommentKey: "other-key", ChapterURL: "old-key"})

	count, err := repo.MigrateKey(testCtx, model.ScopeOfUser(uid), "old-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 migrated, got %d", count)
	}

	// 迁移结果：两条落进新键
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"new-key"}, 1, 10); total != 2 {
		t.Errorf("comments under new-key = %d, want 2", total)
	}
	// 旧键下应无评论
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"old-key"}, 1, 10); total != 0 {
		t.Errorf("comments under old-key = %d, want 0", total)
	}
	// 其他键不受影响
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"other-key"}, 1, 10); total != 2 {
		t.Errorf("comments under other-key = %d, want 2", total)
	}
	// 幂等重试：再跑一次没有匹配行，返回 0 而不是报错
	if retry, err := repo.MigrateKey(testCtx, model.ScopeOfUser(uid), "old-key", "new-key"); err != nil || retry != 0 {
		t.Errorf("retry MigrateKey = (%d, %v), want (0, nil)", retry, err)
	}

	// 废弃列不参与迁移：legacy 的 chapter_url 仍是 old-key，照旧可读
	legacy, legacyTotal, _ := repo.FindByChapterURLs(testCtx, []string{"old-key"}, "", 1, 10)
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
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "c1", CommentKey: "other-key"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "unkeyed"})

	count, err := repo.MigrateKey(testCtx, model.ScopeOfUser(uid), "nonexistent", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey no-match failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 migrated, got %d", count)
	}
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"new-key"}, 1, 10); total != 0 {
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
	userRepo.Create(testCtx, user1)
	userRepo.Create(testCtx, user2)

	repo := NewCommentRepository(testDB)
	repo.Create(testCtx, &model.Comment{UserID: user1.UID, Content: "u1c1", CommentKey: "shared-key"})
	repo.Create(testCtx, &model.Comment{UserID: user1.UID, Content: "u1-unkeyed"})
	repo.Create(testCtx, &model.Comment{UserID: user2.UID, Content: "u2c1", CommentKey: "shared-key"})
	repo.Create(testCtx, &model.Comment{UserID: user2.UID, Content: "u2-unkeyed"})

	// 只迁移 user1 的
	count, err := repo.MigrateKey(testCtx, model.ScopeOfUser(user1.UID), "shared-key", "new-key")
	if err != nil {
		t.Fatalf("MigrateKey user isolation failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 migrated, got %d", count)
	}

	// user2 的评论仍在旧键下
	comments, _, _ := repo.FindByCommentKeys(testCtx, []string{"shared-key"}, 1, 10)
	if len(comments) != 1 || comments[0].UserID != user2.UID {
		t.Errorf("user2's comment should remain under shared-key, got %+v", comments)
	}

	// 空键收拢同样受 user_id 约束：只带走 user1 自己那条未换键的行
	if swept, err := repo.MigrateKey(testCtx, model.ScopeOfUser(user1.UID), "", "ch-1"); err != nil || swept != 1 {
		t.Errorf("empty-old-key sweep = (%d, %v), want (1, nil)", swept, err)
	}
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{""}, 1, 10); total != 1 {
		t.Errorf("user2's unkeyed row must stay unkeyed, empty-key total = %d, want 1", total)
	}
}

// TestCommentRepository_Search 内容关键字 + 书名筛选，零值条件不过滤。
func TestCommentRepository_Search(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "cs@example.com", "cs", "cs")
	repo := NewCommentRepository(testDB)
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "这一章很精彩", BookName: "书A"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "排版有问题", BookName: "书A"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "很精彩的序言", BookName: "书B"})

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
			_, total, err := repo.Search(testCtx, c.query, 1, 10)
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
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "keep"})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "drop"})
	_, _ = repo.DeleteScoped(testCtx, 2, model.CommentScopeOfAdmin())

	count, err := repo.Count(testCtx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Count = %d, want 1 (软删除不得计入)", count)
	}

	perUser, err := repo.CountByUserID(testCtx, uid)
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
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "book-1", CommentKey: ""})
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "book-2", CommentKey: ""})
	// 已有键的行（哪怕带着旧 chapter_url）不能被空键迁移碰
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "chapter-1", CommentKey: "key-a",
		ChapterURL: "https://src.example.com/b/1.html"})

	migrated, err := repo.MigrateKey(testCtx, model.ScopeOfUser(uid), "", "ch-1")
	if err != nil {
		t.Fatalf("MigrateKey failed: %v", err)
	}
	if migrated != 2 {
		t.Errorf("migrated = %d, want 2", migrated)
	}

	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"ch-1"}, 1, 10); total != 2 {
		t.Errorf("comments under ch-1 = %d, want 2", total)
	}
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"key-a"}, 1, 10); total != 1 {
		t.Errorf("unrelated key should be untouched, got %d", total)
	}
	// 收拢后不再有待换键的行
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{""}, 1, 10); total != 0 {
		t.Errorf("unkeyed rows left = %d, want 0", total)
	}
	// 迁移不改写废弃列：那条行仍可按旧 chapter_url 读到
	if _, total, _ := repo.FindByChapterURLs(testCtx, []string{"https://src.example.com/b/1.html"}, "", 1, 10); total != 1 {
		t.Errorf("chapter_url snapshot should stay intact, got %d", total)
	}
}

// TestCommentRepository_MigrateKey_NullKeyedLegacyRows 收拢空键时要连 NULL 那批一起认。
//
// 上面那条用例造不出这个形状：GORM 的 Create 写进去的是空串，而真实升级路径不是这样
// ——database.Migrate 收养旧库时 AutoMigrate 给 comments 补一个不带默认值的 TEXT 列，
// 存量行拿到的是 NULL，而 SQLite 里拿 NULL 与空串作等值比较不成真。只按空串收拢因此
// 在最需要它的库上静默迁移 0 条，用户看到的是「历史评论一条都没合并」。
// 只有 raw SQL 能插入 NULL，这个形状也就只能这样钉住。
func TestCommentRepository_MigrateKey_NullKeyedLegacyRows(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "mignull@example.com", "mignull", "mignull")
	other := seedUser(t, "mignullother@example.com", "mignullother", "mignullother")
	repo := NewCommentRepository(testDB)

	insertNullKeyed := func(owner uint, content string) {
		t.Helper()
		if err := testDB.Exec(
			`INSERT INTO comments (user_id, content, comment_key, created_at) VALUES (?, ?, NULL, CURRENT_TIMESTAMP)`,
			owner, content,
		).Error; err != nil {
			t.Fatalf("seed NULL-keyed row: %v", err)
		}
	}

	// 本人的两种「无键」：升级补列留下的 NULL，以及本服务自己写过的空串
	insertNullKeyed(uid, "legacy-null")
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "legacy-empty", CommentKey: ""})
	// 他人的 NULL 行不在本次收拢范围内
	insertNullKeyed(other, "someone-elses-null")

	migrated, err := repo.MigrateKey(testCtx, model.ScopeOfUser(uid), "", "ch-1")
	if err != nil {
		t.Fatalf("MigrateKey failed: %v", err)
	}
	if migrated != 2 {
		t.Errorf("migrated = %d, want 2（NULL 与空串两种无键形状都要收拢）", migrated)
	}

	var left int64
	if err := testDB.Raw(`SELECT COUNT(*) FROM comments WHERE comment_key IS NULL`).Scan(&left).Error; err != nil {
		t.Fatalf("count NULL-keyed rows: %v", err)
	}
	if left != 1 {
		t.Errorf("NULL-keyed rows left = %d, want 1（只该剩下他人那一行）", left)
	}
}

// TestCommentRepository_MigrateKey_AllUsers 全局改键不限用户，且只动指定的那一桶。
//
// scope 是数据层唯一的区别：不限用户即少一个 user_id 过滤，其余同一条 UPDATE。
// 公开端点的 migrate 只搬本人行，错桶上聚着他人评论时救不了场，因此需要跨用户的
// 改键能力——它只由后台端点触达（ADR-0010）。
func TestCommentRepository_MigrateKey_AllUsers(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	u1 := seedUser(t, "rehash_u1@example.com", "rehash1", "rehash1")
	u2 := seedUser(t, "rehash_u2@example.com", "rehash2", "rehash2")
	repo := NewCommentRepository(testDB)
	repo.Create(testCtx, &model.Comment{UserID: u1, Content: "polluted-1", CommentKey: "ck1:polluted"})
	repo.Create(testCtx, &model.Comment{UserID: u2, Content: "polluted-2", CommentKey: "ck1:polluted"})
	repo.Create(testCtx, &model.Comment{UserID: u1, Content: "elsewhere", CommentKey: "ck1:other"})
	// 废弃列同名的行不该被牵连：改键只认 comment_key 列
	repo.Create(testCtx, &model.Comment{UserID: u1, Content: "legacy", ChapterURL: "ck1:polluted"})

	count, err := repo.MigrateKey(testCtx, model.ScopeAllUsers(), "ck1:polluted", "ck1:fixed")
	if err != nil {
		t.Fatalf("MigrateKey(all users) failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 rows rehashed across both users, got %d", count)
	}
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"ck1:fixed"}, 1, 10); total != 2 {
		t.Errorf("Expected 2 rows under new key, got %d", total)
	}
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"ck1:polluted"}, 1, 10); total != 0 {
		t.Errorf("Expected old bucket emptied, got %d rows left", total)
	}
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"ck1:other"}, 1, 10); total != 1 {
		t.Errorf("Unrelated bucket should be untouched, got %d", total)
	}
	if _, total, _ := repo.FindByChapterURLs(testCtx, []string{"ck1:polluted"}, "", 1, 10); total != 1 {
		t.Errorf("Rehash must not touch the deprecated chapter_url column, got %d", total)
	}
}

// TestCommentRepository_MigrateKey_AllUsersNoMatch 无匹配行返回 0 而非报错（幂等，可重复调用）。
func TestCommentRepository_MigrateKey_AllUsersNoMatch(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewCommentRepository(testDB)
	count, err := repo.MigrateKey(testCtx, model.ScopeAllUsers(), "ck1:missing", "ck1:target")
	if err != nil {
		t.Fatalf("MigrateKey(all users) no-match failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rehashed, got %d", count)
	}
}

// TestCommentRepository_MigrateKey_ScopedToUserZeroMatchesNothing 限定用户但 uid 为 0
// 时必须一行不动。
//
// 「不限用户」是一个独立的 scope 值，不是 userID == 0 这个数：否则一次 uid 为 0 的
// 迁移（token 不指名账号、或调用方漏了校验）就从「匹配 0 行」静默变成「改写全表」，
// 那是只有后台表面才有权做的跨用户写（ADR-0013/0014）。
func TestCommentRepository_MigrateKey_ScopedToUserZeroMatchesNothing(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "zero-scope@example.com", "zs", "zs")
	repo := NewCommentRepository(testDB)
	repo.Create(testCtx, &model.Comment{UserID: uid, Content: "c1", CommentKey: "ck1:shared"})
	second := seedUser(t, "zero-scope2@example.com", "zs2", "zs2")
	repo.Create(testCtx, &model.Comment{UserID: second, Content: "orphan", CommentKey: "ck1:shared"})

	count, err := repo.MigrateKey(testCtx, model.ScopeOfUser(0), "ck1:shared", "ck1:moved")
	if err != nil {
		t.Fatalf("MigrateKey(ScopeOfUser(0)) failed: %v", err)
	}
	if count != 0 {
		t.Errorf("限定 uid=0 改写了 %d 行，应一行不动", count)
	}
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"ck1:shared"}, 1, 10); total != 2 {
		t.Errorf("旧桶应原样留着 2 行（两个用户的），实际 %d 行", total)
	}
	if _, total, _ := repo.FindByCommentKeys(testCtx, []string{"ck1:moved"}, 1, 10); total != 0 {
		t.Errorf("不该有任何行被迁走，实际 %d 行", total)
	}

	// 同一把键在 ScopeAllUsers 下才真的跨用户改写
	all, err := repo.MigrateKey(testCtx, model.ScopeAllUsers(), "ck1:shared", "ck1:moved")
	if err != nil || all != 2 {
		t.Errorf("ScopeAllUsers 应改写 2 行，得 (%d, %v)", all, err)
	}
}
