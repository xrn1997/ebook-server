package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/upload"
	"ebook-server/repository"
)

// writeAvatarFile 在临时 uploads/avatar 下放一个文件，并返回它的公开 URL。
func writeAvatarFile(t *testing.T, store *upload.Store, name string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(store.AvatarDir(), name), []byte("img"), 0o600); err != nil {
		t.Fatalf("write avatar file: %v", err)
	}
	return "http://example.com" + upload.AvatarURLPrefix + "/" + name
}

func TestProfileService_Get_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "getbyiduser@example.com", "password123")

	got, err := testUsers.Get(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if got.Username != user.Username {
		t.Errorf("Expected username '%s', got '%s'", user.Username, got.Username)
	}
}

func TestProfileService_Get_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	if _, err := testUsers.Get(testCtx, 999999); err != model.ErrAccountNotFound {
		t.Errorf("应透出 repository 的 model.ErrAccountNotFound，got %v", err)
	}
}

func TestProfileService_Update_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "old@example.com", "password123")

	updateReq := &model.UpdateUserRequest{
		Username: "newuser",
		Avatar:   "https://example.com/new-avatar.jpg",
	}
	if _, err := testUsers.Update(testCtx, user.UID, updateReq); err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}
	// 从库里读回来断言：Update 返回的是它自己刚改过的那个内存对象，
	// 只判返回值等于请求值，等于什么都没测（列级 UPDATE 写错 uid 也能过）。
	stored, err := testUsers.Get(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Failed to read back user: %v", err)
	}
	if stored.Username != updateReq.Username {
		t.Errorf("Expected username '%s', got '%s'", updateReq.Username, stored.Username)
	}
	if stored.Email != "old@example.com" {
		t.Errorf("Expected email to remain 'old@example.com', got '%s'", stored.Email)
	}
	if stored.Avatar != updateReq.Avatar {
		t.Errorf("Expected avatar '%s', got '%s'", updateReq.Avatar, stored.Avatar)
	}
}

// TestProfileService_Update_EmailImmutable 登录主标识不可变更（ADR-0004）
func TestProfileService_Update_EmailImmutable(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "immutable@example.com", "password123")

	_, err := testUsers.Update(testCtx, user.UID, &model.UpdateUserRequest{Email: "another@example.com"})
	if err != model.ErrEmailImmutable {
		t.Errorf("Expected ErrEmailImmutable, got %v", err)
	}

	got, err := testUsers.Get(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if got.Email != "immutable@example.com" {
		t.Errorf("Expected email unchanged, got '%s'", got.Email)
	}
}

// TestProfileService_Update_EmailSameValue 传入与当前值相同的邮箱应幂等忽略
func TestProfileService_Update_EmailSameValue(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "same@example.com", "password123")

	updated, err := testUsers.Update(testCtx, user.UID, &model.UpdateUserRequest{
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

func TestProfileService_Update_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	if _, err := testUsers.Update(testCtx, 999999, &model.UpdateUserRequest{Nickname: "新昵称"}); err != model.ErrAccountNotFound {
		t.Errorf("应透出 repository 的 model.ErrAccountNotFound，got %v", err)
	}
}

// TestProfileService_Update_ReturnsPersistedUser 返回值必须反映库里的最新行。
//
// 列级 UPDATE 会改写 updated_at；若把调用前读到的对象改几个字段就返回，
// 客户端拿到的 updated_at 永远停在更新前（其它字段却已经变了）。
func TestProfileService_Update_ReturnsPersistedUser(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "persisted@example.com", "password123")
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := testDB.Model(&model.User{}).Where("uid = ?", user.UID).
		UpdateColumn("updated_at", old).Error; err != nil {
		t.Fatalf("seed updated_at: %v", err)
	}

	updated, err := testUsers.Update(testCtx, user.UID, &model.UpdateUserRequest{Nickname: "新昵称"})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	stored, err := testUsers.Get(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Failed to read back user: %v", err)
	}
	if !updated.UpdatedAt.Equal(stored.UpdatedAt) {
		t.Errorf("返回对象的 updated_at 与库不一致：got %v, want %v", updated.UpdatedAt, stored.UpdatedAt)
	}
	if !updated.UpdatedAt.After(old) {
		t.Errorf("updated_at 未刷新：got %v, want after %v", updated.UpdatedAt, old)
	}
}

// TestProfileService_Update_PartialUpdate 空串表示「不修改」，而不是「清空」。
//
// 只递头像时，注册生成的用户名与昵称必须原样留在库里——列级 UPDATE 少写一列
// 或用整行 Save 覆盖，都只会在这里暴露。
func TestProfileService_Update_PartialUpdate(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "original@example.com", "password123")

	if _, err := testUsers.Update(testCtx, user.UID, &model.UpdateUserRequest{
		Avatar: "https://example.com/avatar.jpg",
	}); err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}

	stored, err := testUsers.Get(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Failed to read back user: %v", err)
	}
	if stored.Email != "original@example.com" {
		t.Errorf("Expected email to remain 'original@example.com', got '%s'", stored.Email)
	}
	if stored.Avatar != "https://example.com/avatar.jpg" {
		t.Errorf("Expected avatar to be updated, got '%s'", stored.Avatar)
	}
	if stored.Username != user.Username {
		t.Errorf("未提供的 username 被改写：期望 '%s'，实际 '%s'", user.Username, stored.Username)
	}
	if stored.Nickname != user.Nickname {
		t.Errorf("未提供的 nickname 被改写：期望 '%s'，实际 '%s'", user.Nickname, stored.Nickname)
	}
}

func TestProfileService_Update_ReplacesAvatarDeletesOldFile(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "profile-avatar@example.com", "password123")
	store := upload.New(t.TempDir(), "")
	oldURL := writeAvatarFile(t, store, "old.png")
	if err := testDB.Model(&model.User{}).Where("uid = ?", user.UID).Update("avatar", oldURL).Error; err != nil {
		t.Fatalf("seed avatar: %v", err)
	}
	svc := NewProfileService(repository.NewUserRepository(testDB), store)

	newURL := "http://example.com/uploads/avatar/new.png"
	got, err := svc.Update(testCtx, user.UID, &model.UpdateUserRequest{Avatar: newURL})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if got.Avatar != newURL {
		t.Fatalf("avatar = %q, want %q", got.Avatar, newURL)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "old.png")); !os.IsNotExist(err) {
		t.Fatalf("旧头像文件应被删除, stat err = %v", err)
	}
}

// TestProfileService_Update_KeepsAvatarSharedByAnotherUser 头像 URL 是公开的，
// 别的账号可能仍引用同一个文件；换头像时不能把它删掉。
func TestProfileService_Update_KeepsAvatarSharedByAnotherUser(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	owner := serviceRegister(t, testAuth, "avatar-owner@example.com", "password123")
	other := serviceRegister(t, testAuth, "avatar-other@example.com", "password123")
	store := upload.New(t.TempDir(), "")
	shared := writeAvatarFile(t, store, "shared.png")

	if err := testDB.Model(&model.User{}).
		Where("uid IN ?", []uint{owner.UID, other.UID}).
		Update("avatar", shared).Error; err != nil {
		t.Fatalf("seed shared avatar: %v", err)
	}
	svc := NewProfileService(repository.NewUserRepository(testDB), store)

	if _, err := svc.Update(testCtx, owner.UID, &model.UpdateUserRequest{
		Avatar: "http://example.com/uploads/avatar/owner-new.png",
	}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "shared.png")); err != nil {
		t.Fatalf("other 仍引用共享头像，文件不应被删除: %v", err)
	}
}

// TestProfileService_ClearAvatar_KeepsAvatarSharedByAnotherUser 注销清头像同样只删无人引用的文件。
func TestProfileService_ClearAvatar_KeepsAvatarSharedByAnotherUser(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	owner := serviceRegister(t, testAuth, "avatar-clear-owner@example.com", "password123")
	other := serviceRegister(t, testAuth, "avatar-clear-other@example.com", "password123")
	store := upload.New(t.TempDir(), "")
	shared := writeAvatarFile(t, store, "shared-clear.png")

	if err := testDB.Model(&model.User{}).
		Where("uid IN ?", []uint{owner.UID, other.UID}).
		Update("avatar", shared).Error; err != nil {
		t.Fatalf("seed shared avatar: %v", err)
	}
	svc := NewProfileService(repository.NewUserRepository(testDB), store)

	if err := svc.ClearAvatar(testCtx, owner.UID); err != nil {
		t.Fatalf("ClearAvatar failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "shared-clear.png")); err != nil {
		t.Fatalf("other 仍引用共享头像，文件不应被删除: %v", err)
	}
}

// TestProfileService_Update_KeepsAvatarWhenOnlyHostDiffers 同一文件换了 host 写法
// （localhost ↔ 内网 IP）仍算「同一个头像」，不能把当前用户正在用的文件删掉。
//
// 引用计数包含当前账号自己正是为了这条：UpdateProfile 已把新 URL 写进库，若排除自己，
// 旧文件名就没人引用了。
func TestProfileService_Update_KeepsAvatarWhenOnlyHostDiffers(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "avatar-host@example.com", "password123")
	store := upload.New(t.TempDir(), "")
	oldURL := writeAvatarFile(t, store, "same-file.png")
	if err := testDB.Model(&model.User{}).Where("uid = ?", user.UID).
		Update("avatar", oldURL).Error; err != nil {
		t.Fatalf("seed avatar: %v", err)
	}
	svc := NewProfileService(repository.NewUserRepository(testDB), store)

	newURL := "http://127.0.0.1:9090" + upload.AvatarURLPrefix + "/same-file.png"
	got, err := svc.Update(testCtx, user.UID, &model.UpdateUserRequest{Avatar: newURL})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if got.Avatar != newURL {
		t.Fatalf("avatar = %q, want %q", got.Avatar, newURL)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "same-file.png")); err != nil {
		t.Fatalf("新 URL 仍指向该文件，旧文件不应被删除: %v", err)
	}
}

// TestProfileService_Update_KeepsAvatarReferencedByAnotherHost 外部 host + 本服务
// 头像路径的 URL 不能成为绕过引用计数的后门。
//
// 攻击者可以把别人的头像 URL 改个 host 写进自己的资料（路径相同、文件同一个），
// 再换一次头像触发删除。按整串 URL 计数时这里会得到 0，循环把对方的文件删掉；
// 按文件名计数则能看见受害者那条引用。
func TestProfileService_Update_KeepsAvatarReferencedByAnotherHost(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	victim := serviceRegister(t, testAuth, "avatar-victim@example.com", "password123")
	attacker := serviceRegister(t, testAuth, "avatar-attacker@example.com", "password123")
	store := upload.New(t.TempDir(), "")
	victimURL := writeAvatarFile(t, store, "guarded.png")

	if err := testDB.Model(&model.User{}).Where("uid = ?", victim.UID).
		Update("avatar", victimURL).Error; err != nil {
		t.Fatalf("seed victim avatar: %v", err)
	}
	spoofedURL := "https://evil.example" + upload.AvatarURLPrefix + "/guarded.png"
	if err := testDB.Model(&model.User{}).Where("uid = ?", attacker.UID).
		Update("avatar", spoofedURL).Error; err != nil {
		t.Fatalf("seed attacker avatar: %v", err)
	}
	svc := NewProfileService(repository.NewUserRepository(testDB), store)

	if _, err := svc.Update(testCtx, attacker.UID, &model.UpdateUserRequest{
		Avatar: "https://evil.example" + upload.AvatarURLPrefix + "/unrelated.png",
	}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "guarded.png")); err != nil {
		t.Fatalf("受害者仍引用该文件，不应被攻击者的换头像操作删除: %v", err)
	}
}

// TestProfileService_Update_KeepsAvatarReferencedByCaseVariant Windows/macOS 的
// 文件系统不区分大小写，`GUARDED.PNG` 与 `guarded.png` 是同一个文件。攻击者可以把
// 受害者公开的头像文件名改成大写填进自己的资料，再换头像触发删除；引用计数忽略
// 大小写时，受害者那条引用才会被看见。
func TestProfileService_Update_KeepsAvatarReferencedByCaseVariant(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	victim := serviceRegister(t, testAuth, "avatar-case-victim@example.com", "password123")
	attacker := serviceRegister(t, testAuth, "avatar-case-attacker@example.com", "password123")
	store := upload.New(t.TempDir(), "")
	victimURL := writeAvatarFile(t, store, "guarded-case.png")

	if err := testDB.Model(&model.User{}).Where("uid = ?", victim.UID).
		Update("avatar", victimURL).Error; err != nil {
		t.Fatalf("seed victim avatar: %v", err)
	}
	spoofedURL := "https://evil.example" + upload.AvatarURLPrefix + "/GUARDED-CASE.PNG"
	if err := testDB.Model(&model.User{}).Where("uid = ?", attacker.UID).
		Update("avatar", spoofedURL).Error; err != nil {
		t.Fatalf("seed attacker avatar: %v", err)
	}
	svc := NewProfileService(repository.NewUserRepository(testDB), store)

	if _, err := svc.Update(testCtx, attacker.UID, &model.UpdateUserRequest{
		Avatar: "https://evil.example" + upload.AvatarURLPrefix + "/unrelated-case.png",
	}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "guarded-case.png")); err != nil {
		t.Fatalf("受害者仍引用该文件（大小写变体），不应被攻击者的换头像操作删除: %v", err)
	}
}

// TestProfileService_ClearAvatar_KeepsAvatarReferencedByAnotherHost 注销清头像走的是
// 同一条引用检查：外部 host 的伪造 URL 不能借注销删掉别人的文件。
func TestProfileService_ClearAvatar_KeepsAvatarReferencedByAnotherHost(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	victim := serviceRegister(t, testAuth, "avatar-clear-victim@example.com", "password123")
	attacker := serviceRegister(t, testAuth, "avatar-clear-attacker@example.com", "password123")
	store := upload.New(t.TempDir(), "")
	victimURL := writeAvatarFile(t, store, "guarded-clear.png")

	if err := testDB.Model(&model.User{}).Where("uid = ?", victim.UID).
		Update("avatar", victimURL).Error; err != nil {
		t.Fatalf("seed victim avatar: %v", err)
	}
	spoofedURL := "https://evil.example" + upload.AvatarURLPrefix + "/guarded-clear.png"
	if err := testDB.Model(&model.User{}).Where("uid = ?", attacker.UID).
		Update("avatar", spoofedURL).Error; err != nil {
		t.Fatalf("seed attacker avatar: %v", err)
	}
	svc := NewProfileService(repository.NewUserRepository(testDB), store)

	if err := svc.ClearAvatar(testCtx, attacker.UID); err != nil {
		t.Fatalf("ClearAvatar failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "guarded-clear.png")); err != nil {
		t.Fatalf("受害者仍引用该文件，不应被攻击者的注销流程删除: %v", err)
	}
}

func TestProfileService_ClearAvatar_ClearsColumnAndFile(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "profile-clear@example.com", "password123")
	store := upload.New(t.TempDir(), "")
	oldURL := writeAvatarFile(t, store, "clear.png")
	if err := testDB.Model(&model.User{}).Where("uid = ?", user.UID).Update("avatar", oldURL).Error; err != nil {
		t.Fatalf("seed avatar: %v", err)
	}
	svc := NewProfileService(repository.NewUserRepository(testDB), store)

	if err := svc.ClearAvatar(testCtx, user.UID); err != nil {
		t.Fatalf("ClearAvatar failed: %v", err)
	}
	var got model.User
	if err := testDB.First(&got, user.UID).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	if got.Avatar != "" {
		t.Fatalf("avatar = %q, want empty", got.Avatar)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "clear.png")); !os.IsNotExist(err) {
		t.Fatalf("头像文件应被删除, stat err = %v", err)
	}
}

func TestProfileService_ClearAvatar_NotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	svc := NewProfileService(repository.NewUserRepository(testDB), upload.New(t.TempDir(), ""))
	if err := svc.ClearAvatar(testCtx, 999999); err != model.ErrAccountNotFound {
		t.Errorf("应透出 model.ErrAccountNotFound, got %v", err)
	}
}

// stubAvatarCleaner 头像清理替身：记录被请求删除的 URL，并可注入删除失败。
// 用来钉住「删旧文件失败只告警不阻断」这条不依赖真实文件系统的分支。
type stubAvatarCleaner struct {
	deleted []string
	err     error
}

func (s *stubAvatarCleaner) DeleteAvatar(url string) error {
	s.deleted = append(s.deleted, url)
	return s.err
}

// TestProfileService_Update_AvatarDeleteFailureDoesNotBlock 删旧头像文件失败只告警：
// 头像列此刻已经生效，不能为一个孤儿文件回滚整次资料更新（ADR-0017 §1）。
// 失败降级为 zap.Warn，孤儿文件由运维清理。
func TestProfileService_Update_AvatarDeleteFailureDoesNotBlock(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "avatar-delfail@example.com", "password123")
	oldURL := "http://example.com" + upload.AvatarURLPrefix + "/old.png"
	if err := testDB.Model(&model.User{}).Where("uid = ?", user.UID).
		Update("avatar", oldURL).Error; err != nil {
		t.Fatalf("seed avatar: %v", err)
	}
	cleaner := &stubAvatarCleaner{err: errors.New("delete failed")}
	svc := NewProfileService(repository.NewUserRepository(testDB), cleaner)

	newURL := "http://example.com" + upload.AvatarURLPrefix + "/new.png"
	got, err := svc.Update(testCtx, user.UID, &model.UpdateUserRequest{Avatar: newURL})
	if err != nil {
		t.Fatalf("删旧文件失败不应阻断资料更新, got err = %v", err)
	}
	if got.Avatar != newURL {
		t.Fatalf("avatar = %q, want %q", got.Avatar, newURL)
	}
	if len(cleaner.deleted) != 1 || cleaner.deleted[0] != oldURL {
		t.Fatalf("应尝试删除旧头像 %q 一次, got %v", oldURL, cleaner.deleted)
	}
}

// TestProfileService_Update_ExternalAvatarDoesNotTouchLocalFiles 旧头像是不属于本服务的
// 外部 URL 时，更新照常成功且不动 uploads/avatar 下的任何文件（设计文档 §5.1.4
// 「外部 URL 不删」）。目录里放一个没人引用的文件，删错了这条就会失败。
func TestProfileService_Update_ExternalAvatarDoesNotTouchLocalFiles(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "external-avatar@example.com", "password123")
	if err := testDB.Model(&model.User{}).Where("uid = ?", user.UID).
		Update("avatar", "https://cdn.example.com/external.png").Error; err != nil {
		t.Fatalf("seed avatar: %v", err)
	}
	store := upload.New(t.TempDir(), "")
	writeAvatarFile(t, store, "unrelated.png")
	svc := NewProfileService(repository.NewUserRepository(testDB), store)

	if _, err := svc.Update(testCtx, user.UID, &model.UpdateUserRequest{
		Avatar: "https://cdn.example.com/new.png",
	}); err != nil {
		t.Fatalf("外部头像 URL 不应让资料更新失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "unrelated.png")); err != nil {
		t.Fatalf("外部 URL 不应删掉本服务目录内的文件: %v", err)
	}
}
