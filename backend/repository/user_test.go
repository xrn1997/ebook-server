package repository

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"ebook-server/model"
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

	if err := repo.Create(testCtx, user); err != nil {
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
	repo.Create(testCtx, user)

	found, err := repo.FindByEmail(testCtx, "find@example.com")
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
	_, err := repo.FindByEmail(testCtx, "nonexistent@example.com")
	if !errors.Is(err, model.ErrAccountNotFound) {
		t.Errorf("账号查不到应回 model.ErrAccountNotFound, got %v", err)
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
	repo.Create(testCtx, user)

	found, err := repo.FindByUID(testCtx, user.UID)
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
	_, err := repo.FindByUID(testCtx, 999999)
	if !errors.Is(err, model.ErrAccountNotFound) {
		t.Errorf("账号查不到应回 model.ErrAccountNotFound, got %v", err)
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
	repo.Create(testCtx, user)

	user.Username = "after"
	user.Avatar = "https://example.com/avatar.jpg"
	if err := repo.Update(testCtx, user); err != nil {
		t.Fatalf("Failed to update user: %v", err)
	}

	found, _ := repo.FindByUID(testCtx, user.UID)
	if found.Username != "after" {
		t.Errorf("Expected username 'after', got '%s'", found.Username)
	}
	if found.Avatar != "https://example.com/avatar.jpg" {
		t.Errorf("Expected avatar updated, got '%s'", found.Avatar)
	}
}

// TestUserRepository_UpdateProfile_DoesNotTouchCredentials 资料更新只写展示列：
// 失败登录计数、锁定时间与密码哈希都不该被旧值覆盖。
func TestUserRepository_UpdateProfile_DoesNotTouchCredentials(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	until := time.Now().Add(15 * time.Minute).Truncate(time.Second)
	user := &model.User{
		Email:         "profile@example.com",
		Password:      "hashedpassword",
		Username:      "before",
		Nickname:      "before",
		LoginAttempts: 3,
		LockedUntil:   &until,
	}
	if err := repo.Create(testCtx, user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	username, nickname, avatar := "after", "new-nick", "avatar.png"
	if err := repo.UpdateProfile(testCtx, user.UID, model.ProfileUpdate{
		Username: &username, Nickname: &nickname, Avatar: &avatar,
	}); err != nil {
		t.Fatalf("UpdateProfile failed: %v", err)
	}

	found, err := repo.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("FindByUID failed: %v", err)
	}
	if found.Username != "after" || found.Nickname != "new-nick" || found.Avatar != "avatar.png" {
		t.Errorf("profile columns not updated: %+v", found)
	}
	if found.Password != "hashedpassword" {
		t.Errorf("password clobbered: %q", found.Password)
	}
	if found.LoginAttempts != 3 || found.LockedUntil == nil {
		t.Errorf("login state clobbered: attempts=%d locked=%v", found.LoginAttempts, found.LockedUntil)
	}
	if found.Email != "profile@example.com" {
		t.Errorf("email changed: %q", found.Email)
	}
}

// TestUserRepository_UpdateProfile_OnlyWritesProvidedColumns 未被本次请求提交的列
// 不得出现在 UPDATE 里：并发改昵称与头像时，后写的一方不能把前者覆盖回旧值。
func TestUserRepository_UpdateProfile_OnlyWritesProvidedColumns(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "partial-profile@example.com",
		Password: "hashedpassword",
		Username: "keep-name",
		Nickname: "keep-nick",
		Avatar:   "keep-avatar.png",
	}
	if err := repo.Create(testCtx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	newNick := "only-nick"
	if err := repo.UpdateProfile(testCtx, user.UID, model.ProfileUpdate{Nickname: &newNick}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	found, err := repo.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("FindByUID: %v", err)
	}
	if found.Nickname != newNick {
		t.Errorf("nickname = %q, want %q", found.Nickname, newNick)
	}
	if found.Username != "keep-name" || found.Avatar != "keep-avatar.png" {
		t.Errorf("unsupplied columns were overwritten: %+v", found)
	}
}

func TestUserRepository_UpdateAvatar_OnlyAvatar(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "avatar@example.com",
		Password: "hashedpassword",
		Username: "u",
		Nickname: "u",
		Avatar:   "old.png",
	}
	if err := repo.Create(testCtx, user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if err := repo.UpdateAvatar(testCtx, user.UID, ""); err != nil {
		t.Fatalf("UpdateAvatar failed: %v", err)
	}

	found, err := repo.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("FindByUID failed: %v", err)
	}
	if found.Avatar != "" {
		t.Errorf("avatar = %q, want empty", found.Avatar)
	}
	if found.Username != "u" || found.Password != "hashedpassword" {
		t.Errorf("other columns touched: %+v", found)
	}
}

// TestUserRepository_CredentialWrites_OnlyCredentialColumns 凭据写路径各只碰自己的列。
//
// 这几条 UPDATE 是 ADR-0017 的落点：失败记账一条语句做完（递增、按阈值上锁、
// 上锁即清零），登录成功把计数与锁定一起抹掉，改密不碰资料。
// 任何一条退回整行 Save，都会在与资料更新并发时把对方写回旧值；
// 任何一条退回「先写后读再判」，锁定判定就不再是数据库说的那一句。
func TestUserRepository_CredentialWrites_OnlyCredentialColumns(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	user := &model.User{
		Email:    "creds@example.com",
		Password: "old-hash",
		Username: "keep-me",
		Nickname: "keep-me",
		Avatar:   "keep.png",
	}
	if err := repo.Create(testCtx, user); err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// 失败记账：达到阈值与否的判定必须由数据库在同一条语句里给出。
	// 「先递增再读回」的两步走会把并发的成功清零读进本次判定——该锁的没锁、
	// 不该锁的提前锁，而 service 层承诺的是原子。
	until := time.Now().Add(15 * time.Minute).Truncate(time.Second)
	for i := 1; i <= 4; i++ {
		locked, err := repo.RecordFailedLogin(testCtx, user.UID, 5, until)
		if err != nil {
			t.Fatalf("RecordFailedLogin 第 %d 次: %v", i, err)
		}
		if locked {
			t.Fatalf("第 %d 次就报告锁定，阈值是 5", i)
		}
	}
	rowing, err := repo.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("FindByUID: %v", err)
	}
	if rowing.LoginAttempts != 4 {
		t.Errorf("4 次失败后 login_attempts = %d, want 4", rowing.LoginAttempts)
	}
	if rowing.LockedUntil != nil {
		t.Errorf("未达阈值不该上锁，实际 locked_until = %v", rowing.LockedUntil)
	}

	locked, err := repo.RecordFailedLogin(testCtx, user.UID, 5, until)
	if err != nil {
		t.Fatalf("RecordFailedLogin 第 5 次: %v", err)
	}
	if !locked {
		t.Error("达到阈值那次必须报告锁定")
	}
	lockedRow, err := repo.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("FindByUID: %v", err)
	}
	if lockedRow.LoginAttempts != 0 {
		t.Errorf("锁定应清零失败计数，实际 %d", lockedRow.LoginAttempts)
	}
	if lockedRow.LockedUntil == nil || !lockedRow.LockedUntil.Truncate(time.Second).Equal(until) {
		t.Errorf("LockedUntil = %v, want %v", lockedRow.LockedUntil, until)
	}
	if lockedRow.Username != "keep-me" || lockedRow.Avatar != "keep.png" || lockedRow.Password != "old-hash" {
		t.Errorf("凭据写路径碰了别的列: %+v", lockedRow)
	}

	// 记在不存在的账号上必须报错：静默回 (false, nil) 等于给暴力破解免掉锁定
	if _, err := repo.RecordFailedLogin(testCtx, 999999, 5, until); !errors.Is(err, model.ErrAccountNotFound) {
		t.Errorf("未知 uid 应回 ErrAccountNotFound, got %v", err)
	}

	if err := repo.UpdatePasswordHash(testCtx, user.UID, "new-hash"); err != nil {
		t.Fatalf("UpdatePasswordHash: %v", err)
	}
	rotated, err := repo.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("FindByUID: %v", err)
	}
	if rotated.Password != "new-hash" {
		t.Errorf("password = %q, want new-hash", rotated.Password)
	}
	if rotated.Username != "keep-me" || rotated.Avatar != "keep.png" {
		t.Errorf("改密碰了资料列: %+v", rotated)
	}

	if err := repo.ClearLoginFailures(testCtx, user.UID); err != nil {
		t.Fatalf("ClearLoginFailures: %v", err)
	}
	cleared, err := repo.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("FindByUID: %v", err)
	}
	if cleared.LoginAttempts != 0 || cleared.LockedUntil != nil {
		t.Errorf("登录成功后应同时清掉计数与锁定，实际 attempts=%d locked=%v",
			cleared.LoginAttempts, cleared.LockedUntil)
	}
}

// TestUserRepository_CountByAvatar 按文件的 URL 文件名统计引用，而不是整串 URL。
//
// 删旧头像文件前的那道引用检查全靠它：同一文件可能以 localhost / 内网 IP 两种 host
// 写进不同账号的资料，按整串比较会漏掉引用，把别人正在用的头像删掉。计数包含
// 当前账号：调用方在写入新 URL 之后才查询，新旧只是 host 不同时这一票负责保文件。
func TestUserRepository_CountByAvatar(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	const sharedA = "http://192.168.1.10:9090/uploads/avatar/shared.png"
	const sharedB = "http://127.0.0.1:9090/uploads/avatar/shared.png"
	owner := &model.User{Email: "owner@example.com", Password: "h", Username: "o", Nickname: "o", Avatar: sharedA}
	other := &model.User{Email: "other@example.com", Password: "h", Username: "t", Nickname: "t", Avatar: sharedB}
	solo := &model.User{Email: "solo@example.com", Password: "h", Username: "s", Nickname: "s", Avatar: "mine.png"}
	for _, u := range []*model.User{owner, other, solo} {
		if err := repo.Create(testCtx, u); err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
	}

	// 两个不同 host 的 URL 指向同一文件名：两个账号都要算进来
	refs, err := repo.CountByAvatar(testCtx, sharedA)
	if err != nil {
		t.Fatalf("CountByAvatar: %v", err)
	}
	if refs != 2 {
		t.Errorf("同名文件的引用者应为 2，实际 %d", refs)
	}
	refs, err = repo.CountByAvatar(testCtx, sharedB)
	if err != nil {
		t.Fatalf("CountByAvatar: %v", err)
	}
	if refs != 2 {
		t.Errorf("换一个 host 写法仍应数到 2 个引用，实际 %d", refs)
	}

	// Windows/macOS 的文件系统不区分大小写：大小写变体仍是同一个文件，必须算引用。
	// 否则攻击者把别人的头像文件名改成大写填进自己的资料，再换头像就能删掉对方文件。
	refs, err = repo.CountByAvatar(testCtx, "http://example.com/uploads/avatar/SHARED.PNG")
	if err != nil {
		t.Fatalf("CountByAvatar: %v", err)
	}
	if refs != 2 {
		t.Errorf("大小写变体应数到 2 个引用，实际 %d", refs)
	}

	// Windows 忽略尾随的点与空格：`shared.png.` 打开的是同一个文件，必须算引用，
	// 否则删除入口拒绝这类名字、计数又漏掉它，文件仍被引用时会被删掉。
	refs, err = repo.CountByAvatar(testCtx, "http://example.com/uploads/avatar/shared.png.")
	if err != nil {
		t.Fatalf("CountByAvatar: %v", err)
	}
	if refs != 2 {
		t.Errorf("尾随点变体应数到 2 个引用，实际 %d", refs)
	}

	// 文件名不匹配：文件才可以删
	refs, err = repo.CountByAvatar(testCtx, "http://example.com/uploads/avatar/nobody.png")
	if err != nil {
		t.Fatalf("CountByAvatar: %v", err)
	}
	if refs != 0 {
		t.Errorf("无人引用的文件应返回 0，实际 %d", refs)
	}

	// 无法解析出文件名的 URL 不得把空名与空名匹配成引用
	refs, err = repo.CountByAvatar(testCtx, "")
	if err != nil {
		t.Fatalf("CountByAvatar: %v", err)
	}
	if refs != 0 {
		t.Errorf("空 URL 应返回 0，实际 %d", refs)
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
	repo.Create(testCtx, user)

	exists, err := repo.ExistsByEmail(testCtx, "exists@example.com")
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
	exists, err := repo.ExistsByEmail(testCtx, "ghost@example.com")
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
	repo.Create(testCtx, user)

	// 设置锁定状态
	user.LoginAttempts = 3
	until := time.Now().Add(15 * time.Minute)
	user.LockedUntil = &until
	repo.Update(testCtx, user)

	found, _ := repo.FindByUID(testCtx, user.UID)
	if found.LoginAttempts != 3 {
		t.Errorf("Expected LoginAttempts 3, got %d", found.LoginAttempts)
	}
	if found.LockedUntil == nil {
		t.Error("Expected LockedUntil to be set")
	}
}

// TestUserRepository_Search 关键字命中邮箱/用户名/昵称，纯数字时兼命中 UID。
func TestUserRepository_Search(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	uid := seedUser(t, "alice@example.com", "alice", "小艾")
	seedUser(t, "bob@example.com", "bob", "阿鲍")

	cases := []struct {
		name    string
		keyword string
		want    int64
	}{
		{"按邮箱", "alice@example", 1},
		{"按昵称", "小艾", 1},
		{"按 UID 数字", strconv.FormatUint(uint64(uid), 10), 1},
		{"空关键字即全量", "", 2},
		{"通配符不被当模式", "%", 0},
		{"无匹配", "zzz", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, total, err := repo.Search(testCtx, c.keyword, 1, 10)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			if total != c.want {
				t.Errorf("Search(%q) total = %d, want %d", c.keyword, total, c.want)
			}
		})
	}
}
