package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/testdb"
	"ebook-server/repository"
)

func newCredentialsForTest(t *testing.T) (*Credentials, *repository.UserRepository, *repository.RefreshTokenRepository) {
	t.Helper()
	db := testdb.Open(t)
	users := repository.NewUserRepository(db)
	tokens := repository.NewRefreshTokenRepository(db)
	return NewCredentials(users, NewSessions(tokens, users)), users, tokens
}

func TestCredentials_CreateWithPassword_Hashes(t *testing.T) {
	creds, users, _ := newCredentialsForTest(t)
	user := &model.User{Email: "cred-create@example.com", Username: "u", Nickname: "u"}
	if err := creds.CreateWithPassword(testCtx, user, "password123"); err != nil {
		t.Fatalf("CreateWithPassword: %v", err)
	}
	got, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("FindByUID: %v", err)
	}
	if got.Password == "password123" || got.Password == "" {
		t.Fatalf("password not hashed: %q", got.Password)
	}
	if !creds.checkPassword(got.Password, "password123") {
		t.Fatal("checkPassword should accept the original password")
	}
}

// TestCredentials_RecordFailedLogin_DoesNotClobberProfile 失败登录只写凭据列。
//
// 这里能钉住的是「写进去的列集合」：资料与密码哈希原样保留、计数按预期归零并锁定。
// 「并发丢更新」由实现形态保证——RecordFailedLogin 只接 uid，做一次原子 UPDATE，
// 手上没有任何可能过期的整行快照；列级 SQL 本身由 repository 的用例对着真库验。
func TestCredentials_RecordFailedLogin_DoesNotClobberProfile(t *testing.T) {
	creds, users, _ := newCredentialsForTest(t)
	user := &model.User{Email: "cred-lock@example.com", Username: "old", Nickname: "old"}
	if err := creds.CreateWithPassword(testCtx, user, "password123"); err != nil {
		t.Fatalf("create: %v", err)
	}
	username, nickname, avatar := "new-name", "new-nick", "avatar.png"
	if err := users.UpdateProfile(testCtx, user.UID, model.ProfileUpdate{
		Username: &username, Nickname: &nickname, Avatar: &avatar,
	}); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	var locked bool
	var err error
	for i := 0; i < 5; i++ {
		locked, err = creds.RecordFailedLogin(testCtx, user.UID)
		if err != nil {
			t.Fatalf("RecordFailedLogin %d: %v", i+1, err)
		}
	}
	if !locked {
		t.Fatal("第 5 次失败应触发锁定")
	}
	got, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Username != "new-name" || got.Nickname != "new-nick" || got.Avatar != "avatar.png" {
		t.Fatalf("失败登录覆盖了资料字段: %+v", got)
	}
	if got.Password != user.Password {
		t.Fatalf("失败登录覆盖了密码哈希: %q", got.Password)
	}
	if got.LockedUntil == nil {
		t.Fatal("LockedUntil should be set")
	}
	// 锁定即清零计数：锁定期结束后还剩完整的 5 次机会，而不是「一失败就再锁 15 分钟」
	if got.LoginAttempts != 0 {
		t.Fatalf("锁定时应重置失败计数，实际为 %d", got.LoginAttempts)
	}
}

// failingTokenStore 让撤销会话失败，用来钉住 fail-closed 顺序：
// 撤销失败时密码必须保持原样。
type failingTokenStore struct {
	refreshTokenStore
}

func (failingTokenStore) DeleteByUserID(_ context.Context, _ uint) error {
	return errors.New("revoke failed")
}

func TestCredentials_Reset_KeepsOldHashWhenRevokeFails(t *testing.T) {
	db := testdb.Open(t)
	users := repository.NewUserRepository(db)
	creds := NewCredentials(users, NewSessions(failingTokenStore{}, users))
	user := &model.User{Email: "cred-fail@example.com", Username: "u", Nickname: "u"}
	if err := creds.CreateWithPassword(testCtx, user, "old-password"); err != nil {
		t.Fatalf("create: %v", err)
	}
	before, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	if err := creds.Reset(testCtx, user.UID, "new-password"); err == nil {
		t.Fatal("Reset should fail when revoking tokens fails")
	}
	after, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if after.Password != before.Password {
		t.Fatal("撤销失败后密码哈希不应被修改")
	}
}

// TestCredentials_Change_RevokesThenWrites 改密编排归凭据模块：验旧口令、撤销旧会话、写新哈希。
//
// 两条断言缺一不可：旧 refresh token 必须失效（否则改密拦不住已经泄漏的会话），
// 新哈希必须立刻可用（否则用户撤销了自己的会话却登不进去）。
func TestCredentials_Change_RevokesThenWrites(t *testing.T) {
	creds, users, tokens := newCredentialsForTest(t)
	user := &model.User{Email: "cred-change@example.com", Username: "u", Nickname: "u"}
	if err := creds.CreateWithPassword(testCtx, user, "old-password"); err != nil {
		t.Fatalf("create: %v", err)
	}
	rec := &model.RefreshToken{TokenHash: "hash", UserID: user.UID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := tokens.Create(testCtx, rec); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	if err := creds.Change(testCtx, user.UID, "old-password", "new-password"); err != nil {
		t.Fatalf("Change: %v", err)
	}
	if _, err := tokens.FindByHash(testCtx, "hash"); err != model.ErrLoginExpired {
		t.Fatalf("改密后旧 refresh token 应被撤销, got %v", err)
	}
	got, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !creds.checkPassword(got.Password, "new-password") {
		t.Fatal("新哈希应立刻可用")
	}
	if creds.checkPassword(got.Password, "old-password") {
		t.Fatal("旧口令应失效")
	}
}

// TestCredentials_Change_FailsClosedWhenRevokeFails 撤销失败则中止：不写新哈希，
// 旧口令与旧会话保持原样——绝不出现「新口令生效、旧 refresh token 仍活着」的中间态。
func TestCredentials_Change_FailsClosedWhenRevokeFails(t *testing.T) {
	db := testdb.Open(t)
	users := repository.NewUserRepository(db)
	creds := NewCredentials(users, NewSessions(failingTokenStore{}, users))
	user := &model.User{Email: "cred-change-fail@example.com", Username: "u", Nickname: "u"}
	if err := creds.CreateWithPassword(testCtx, user, "old-password"); err != nil {
		t.Fatalf("create: %v", err)
	}
	before, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	if err := creds.Change(testCtx, user.UID, "old-password", "new-password"); err == nil {
		t.Fatal("撤销失败时 Change 应返回错误")
	}
	after, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if after.Password != before.Password {
		t.Fatal("撤销失败后密码哈希不应被修改")
	}
	if !creds.checkPassword(after.Password, "old-password") {
		t.Fatal("旧口令必须保持可用")
	}
}

// TestCredentials_Reset_RevokesThenWrites 重置编排归凭据模块（验证码校验留在认证模块）：
// 撤销旧会话、写新哈希，断言与 Change 同款。
func TestCredentials_Reset_RevokesThenWrites(t *testing.T) {
	creds, users, tokens := newCredentialsForTest(t)
	user := &model.User{Email: "cred-reset@example.com", Username: "u", Nickname: "u"}
	if err := creds.CreateWithPassword(testCtx, user, "old-password"); err != nil {
		t.Fatalf("create: %v", err)
	}
	rec := &model.RefreshToken{TokenHash: "hash", UserID: user.UID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := tokens.Create(testCtx, rec); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	if err := creds.Reset(testCtx, user.UID, "new-password"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := tokens.FindByHash(testCtx, "hash"); err != model.ErrLoginExpired {
		t.Fatalf("重置后旧 refresh token 应被撤销, got %v", err)
	}
	got, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !creds.checkPassword(got.Password, "new-password") {
		t.Fatal("重置后的新哈希应可用")
	}
	if creds.checkPassword(got.Password, "old-password") {
		t.Fatal("旧口令应失效")
	}
}

// TestIsLoginDenied 折叠规则钉在四枚哨兵上：登录拒绝在 handler 只判一次就给同一信封。
//
// 负例是这条规则的另一半——折叠面必须是闭集；放宽到别的哨兵，
// handler 的一处判断就会把无关错误也折成 A0210，掩盖真实故障。
func TestIsLoginDenied(t *testing.T) {
	for _, err := range []error{
		model.ErrAccountNotFound,
		model.ErrPasswordWrong,
		model.ErrAttemptTooMany,
		model.ErrAccountLocked,
	} {
		if !IsLoginDenied(err) {
			t.Errorf("IsLoginDenied(%v) = false，登录拒绝哨兵必须归为同一类", err)
		}
	}

	for _, err := range []error{
		model.ErrLoginExpired, // 刷新/登出面（A0230），不是登录凭据拒绝
		model.ErrEmailExists,  // 注册面（A0112）
		model.ErrCodeInvalid,  // 验证码面（A0132）
		errors.New("数据库连不上"),
	} {
		if IsLoginDenied(err) {
			t.Errorf("IsLoginDenied(%v) = true，非登录拒绝错误不得折叠", err)
		}
	}
	if IsLoginDenied(nil) {
		t.Error("IsLoginDenied(nil) 应为 false")
	}
}

// TestCredentials_RejectsPasswordOverBcryptLimit 密码的字节上限由凭据模块兜住。
//
// request 结构体的 binding 用 max=50 按**字符**计数，而 bcrypt 只接受至多 72 字节：
// 25 个汉字（75 字节）能通过 handler 校验，却在 GenerateFromPassword 处失败；未登记
// 的 ErrPasswordTooLong 会让注册/改密/重置统统落成 C0500。
func TestCredentials_RejectsPasswordOverBcryptLimit(t *testing.T) {
	creds, users, _ := newCredentialsForTest(t)
	tooLong := strings.Repeat("密", 25) // 25 个字符、75 字节：binding 允许，bcrypt 不允许

	user := &model.User{Email: "cred-too-long@example.com", Username: "u", Nickname: "u"}
	if err := creds.CreateWithPassword(testCtx, user, tooLong); !errors.Is(err, model.ErrPasswordInvalid) {
		t.Fatalf("CreateWithPassword err = %v, want ErrPasswordInvalid", err)
	}
	if _, err := users.FindByEmail(testCtx, user.Email); err == nil {
		t.Fatal("超长密码的账号不应落库")
	}

	// 恰好 72 字节（24 个汉字）仍要能用：上限是字节数，不是字符数
	ok := &model.User{Email: "cred-72@example.com", Username: "u", Nickname: "u"}
	if err := creds.CreateWithPassword(testCtx, ok, strings.Repeat("密", 24)); err != nil {
		t.Fatalf("72 字节密码应被接受: %v", err)
	}
	// 改密/重置共用 setPassword，同一约束必须同样生效
	if err := creds.Reset(testCtx, ok.UID, tooLong); !errors.Is(err, model.ErrPasswordInvalid) {
		t.Fatalf("Reset err = %v, want ErrPasswordInvalid", err)
	}
}

func TestCredentials_ClearFailures(t *testing.T) {
	creds, users, _ := newCredentialsForTest(t)
	user := &model.User{Email: "cred-clear@example.com", Username: "u", Nickname: "u"}
	if err := creds.CreateWithPassword(testCtx, user, "password123"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := creds.RecordFailedLogin(testCtx, user.UID); err != nil {
		t.Fatalf("RecordFailedLogin: %v", err)
	}
	if err := creds.ClearFailures(testCtx, user.UID); err != nil {
		t.Fatalf("ClearFailures: %v", err)
	}
	got, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.LoginAttempts != 0 || got.LockedUntil != nil {
		t.Fatalf("failures not cleared: attempts=%d locked=%v", got.LoginAttempts, got.LockedUntil)
	}
}

// failingCredentialStore 让清零失败计数失败，用来钉住错误向调用方传播（设计文档 §5.2.4）：
// 登录成功后清零失败若被吞掉，调用方会以为计数已经归零。
type failingCredentialStore struct {
	CredentialStore
}

func (failingCredentialStore) ClearLoginFailures(_ context.Context, _ uint) error {
	return errors.New("clear failed")
}

func TestCredentials_ClearFailures_PropagatesError(t *testing.T) {
	users := repository.NewUserRepository(testdb.Open(t))
	creds := NewCredentials(failingCredentialStore{}, NewSessions(failingTokenStore{}, users))
	if err := creds.ClearFailures(testCtx, 1); err == nil {
		t.Fatal("ClearFailures 应把存储层错误透给调用方，不能吞掉")
	}
}
