package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"ebook-server/model"
	"ebook-server/pkg/upload"
	"ebook-server/repository"
)

// TestAuthService_SendDeletionCode 注销发码经真实邮件链路下发，并按本流程配额限流（ADR-0005）
func TestAuthService_SendDeletionCode(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "del-code@example.com", "password123")

	if err := testAccount.SendDeletionCode(testCtx, user.UID); err != nil {
		t.Fatalf("Failed to send deletion code: %v", err)
	}

	// 码由模块下发、命名空间是它的私有细节（跨流程不通用见 verification_test.go）
	if testMails.last("del-code@example.com") == "" {
		t.Error("Expected a deletion code dispatched by mail")
	}
	// 注销流程自己的配额：窗口内连发第二次被挡
	if err := testAccount.SendDeletionCode(testCtx, user.UID); err != model.ErrAttemptTooMany {
		t.Errorf("Expected the deletion-flow send limit, got %v", err)
	}
}

// TestAuthService_DeleteAccount_Anonymize 注销后账号被匿名化（ADR-0005）
func TestAuthService_DeleteAccount_Anonymize(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "anon@example.com", "password123")

	export, err := testAccount.Delete(testCtx, user.UID, seedCode(t, FlowDeletion, "anon@example.com"))
	if err != nil {
		t.Fatalf("Failed to delete account: %v", err)
	}

	// 导出应保留注销前的真实资料，而不是占位值
	if export.User.Email != "anon@example.com" {
		t.Errorf("Expected exported email 'anon@example.com', got '%s'", export.User.Email)
	}

	anon, err := testAccount.users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Failed to find anonymized user: %v", err)
	}

	if anon.Email != "deleted_"+strconv.Itoa(int(user.UID))+"@invalid" {
		t.Errorf("Expected placeholder email, got '%s'", anon.Email)
	}
	if anon.Password != "" {
		t.Errorf("Expected password cleared, got '%s'", anon.Password)
	}
	if anon.Avatar != "" {
		t.Errorf("Expected avatar cleared, got '%s'", anon.Avatar)
	}
	if anon.Username == user.Username {
		t.Error("Expected username to be replaced")
	}
	// 关键：不能软删，否则 GORM 的 Preload 会过滤掉评论作者
	if anon.DeletedAt.Valid {
		t.Error("Expected DeletedAt to stay NULL — soft delete would break comment author preloading")
	}
}

// TestAuthService_DeleteAccount_EmailReusable 注销后同一邮箱可以重新注册（ADR-0005）
func TestAuthService_DeleteAccount_EmailReusable(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "reuse@example.com", "password123")

	if _, err := testAccount.Delete(testCtx, user.UID, seedCode(t, FlowDeletion, "reuse@example.com")); err != nil {
		t.Fatalf("Failed to delete account: %v", err)
	}

	// 同一邮箱重新注册应成功——占位 email 已释放唯一索引
	reborn := serviceRegister(t, authService, "reuse@example.com", "newpassword123")
	if reborn.UID == user.UID {
		t.Errorf("Expected a new uid after re-registration, got same uid %d", reborn.UID)
	}
	if reborn.Email != "reuse@example.com" {
		t.Errorf("Expected new account to hold the email, got '%s'", reborn.Email)
	}
}

// serviceLogin 登录并返回签发的双 token
func serviceLogin(t *testing.T, authService *AuthService, email, password string) *model.TokenPair {
	t.Helper()
	pair, err := authService.Login(testCtx, &model.LoginRequest{Email: email, Password: password})
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	return pair
}

// TestAuthService_DeleteAccount_RevokesRefreshTokens 注销后 refresh token 全部失效（ADR-0005）
//
// ADR-0005「冲突」：注销保留 users 行，故 refresh_tokens 的外键级联删除不再发生，
// 必须由 DeleteAccount 显式删除该用户的全部 refresh token。
func TestAuthService_DeleteAccount_RevokesRefreshTokens(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "revoke@example.com", "password123")

	// 登两次拿到两枚独立的 refresh token：
	// 第一枚用于注销前的冒烟验证，第二枚用于注销后的失效验证。
	// 不能复用同一枚——Refresh 是轮换制，用过一次旧 token 即作废。
	before := serviceLogin(t, authService, "revoke@example.com", "password123")
	after := serviceLogin(t, authService, "revoke@example.com", "password123")

	// 冒烟：注销前该 token 可用。否则下面的失效断言即使通过也无意义。
	if _, err := authService.Refresh(testCtx, before.RefreshToken); err != nil {
		t.Fatalf("Refresh should succeed before deletion: %v", err)
	}

	if _, err := testAccount.Delete(testCtx, user.UID, seedCode(t, FlowDeletion, "revoke@example.com")); err != nil {
		t.Fatalf("Failed to delete account: %v", err)
	}

	if _, err := authService.Refresh(testCtx, after.RefreshToken); err != model.ErrLoginExpired {
		t.Errorf("Refresh should fail with ErrLoginExpired after deletion, got: %v", err)
	}
}

// rotationRaceStore 在轮换消费旧令牌的瞬间插入一段回调，用于复现注销与并发刷新
// 的竞态最坏交错：Refresh 已消费旧令牌、尚未写入新令牌时，注销的全部写恰好完成。
type rotationRaceStore struct {
	TokenStore
	onConsumed func()
}

func (s *rotationRaceStore) DeleteByID(ctx context.Context, id uint) (bool, error) {
	consumed, err := s.TokenStore.DeleteByID(ctx, id)
	if consumed && s.onConsumed != nil {
		hook := s.onConsumed
		s.onConsumed = nil // 只插一次
		hook()
	}
	return consumed, err
}

// TestAuthService_Refresh_RacingDeleteIssuesOnlyDeadTokens 注销与并发刷新的竞态不留活口。
//
// 竞态最坏交错：Refresh 消费旧令牌之后、写入新令牌之前，注销的全部写完成——新令牌
// 漏过 DeleteByUserID 留在库里。断言两层：竞态窗口内的轮换照常完成（签发端无法感知
// 刚发生的注销），但这枚漏网的令牌不得再续期——否则一条会话能在账号注销后无限续期，
// 继续以「已注销用户_x」发言（Refresh 对匿名化账号一律 ErrAccountNotFound）。
func TestAuthService_Refresh_RacingDeleteIssuesOnlyDeadTokens(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	user := serviceRegister(t, testAuth, "race@example.com", "password123")
	pair := serviceLogin(t, testAuth, "race@example.com", "password123")

	users := repository.NewUserRepository(testDB)
	tokensRepo := repository.NewRefreshTokenRepository(testDB)
	raceStore := &rotationRaceStore{TokenStore: tokensRepo}
	raceAuth := NewAuthService(
		users,
		raceStore,
		NewVerificationCodes(testCodes, testMails),
		testTokens,
		NewCredentials(users, raceStore),
	)

	// 在轮换消费旧令牌的瞬间跑完整个注销：新令牌的写入将落在注销之后
	raceStore.onConsumed = func() {
		if _, err := testAccount.Delete(testCtx, user.UID, seedCode(t, FlowDeletion, "race@example.com")); err != nil {
			t.Errorf("注销在刷新窗口内执行失败: %v", err)
		}
	}

	payload, err := raceAuth.Refresh(testCtx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("竞态窗口内的轮换应照常完成: %v", err)
	}

	if _, err := raceAuth.Refresh(testCtx, payload.RefreshToken); err != model.ErrAccountNotFound {
		t.Errorf("注销后漏网的 refresh token 应被拒绝 (ErrAccountNotFound), got: %v", err)
	}
}

// TestAuthService_DeleteAccount_WrongCode 验证码错误时应拒绝注销，账号保持原样
func TestAuthService_DeleteAccount_WrongCode(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "wrongcode@example.com", "password123")

	wrong := seedCodeWrong(t, FlowDeletion, "wrongcode@example.com")
	if _, err := testAccount.Delete(testCtx, user.UID, wrong); err != model.ErrCodeInvalid {
		t.Errorf("Expected ErrCodeInvalid, got %v", err)
	}

	got, err := testAccount.users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Failed to find user: %v", err)
	}
	if got.Email != "wrongcode@example.com" {
		t.Errorf("Expected account untouched, got email '%s'", got.Email)
	}
}

// TestAuthService_DeleteAccount_ExportsComments 注销响应应带上用户的评论
func TestAuthService_DeleteAccount_ExportsComments(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "export@example.com", "password123")

	commentService := testComments
	if _, err := commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{
		Content:     "我的一条评论",
		CommentKey:  "ck1:export",
		ChapterName: "第一章",
		BookName:    "示例之书",
	}); err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	export, err := testAccount.Delete(testCtx, user.UID, seedCode(t, FlowDeletion, "export@example.com"))
	if err != nil {
		t.Fatalf("Failed to delete account: %v", err)
	}

	if len(export.Comments) != 1 {
		t.Fatalf("Expected 1 exported comment, got %d", len(export.Comments))
	}
	if export.Comments[0].Content != "我的一条评论" {
		t.Errorf("Expected exported comment content, got '%s'", export.Comments[0].Content)
	}
	if export.Comments[0].CommentKey != "ck1:export" ||
		export.Comments[0].ChapterName != "第一章" ||
		export.Comments[0].BookName != "示例之书" {
		t.Errorf("export lost comment context: %+v", export.Comments[0])
	}
}

// TestAccountService_Delete_RemovesAvatarFile 注销后头像文件一并删除（ADR-0011 的生命周期缺口）。
//
// 导出副本在清空之前构建，因此仍包含注销前的头像 URL；磁盘文件则不得再被公开路由服务。
func TestAccountService_Delete_RemovesAvatarFile(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	store := upload.New(t.TempDir(), "")
	users := repository.NewUserRepository(testDB)
	profile := NewProfileService(users, store)
	account := NewAccountService(
		users,
		repository.NewRefreshTokenRepository(testDB),
		repository.NewCommentRepository(testDB),
		NewVerificationCodes(testCodes, &recordingMailer{}),
		profile,
	)

	email := "delete-avatar@example.com"
	user := serviceRegister(t, testAuth, email, "password123")
	avatarURL := writeAvatarFile(t, store, "delete.png")
	if _, err := profile.Update(testCtx, user.UID, &model.UpdateUserRequest{Avatar: avatarURL}); err != nil {
		t.Fatalf("seed avatar: %v", err)
	}

	export, err := account.Delete(testCtx, user.UID, seedCode(t, FlowDeletion, email))
	if err != nil {
		t.Fatalf("Failed to delete account: %v", err)
	}
	if export.User.Avatar != avatarURL {
		t.Errorf("Expected export to keep pre-deletion avatar %q, got %q", avatarURL, export.User.Avatar)
	}
	if _, err := os.Stat(filepath.Join(store.AvatarDir(), "delete.png")); !os.IsNotExist(err) {
		t.Fatalf("注销后头像文件应被删除, stat err = %v", err)
	}
}

// TestAccountService_Delete_ReturnsExportWhenTokenRevokeFails 令牌清理失败不能让用户丢数据。
//
// 走到这一步时账号已经匿名化、数据副本也已经构建；把清理失败判成整体失败，用户就
// 再也拿不到那份唯一的副本（匿名化后无法登录重试）。漏网令牌本身续不了期，所以
// 这里只告警并照常返回 export。
func TestAccountService_Delete_ReturnsExportWhenTokenRevokeFails(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	users := repository.NewUserRepository(testDB)
	account := NewAccountService(
		users,
		failingTokenStore{TokenStore: repository.NewRefreshTokenRepository(testDB)},
		repository.NewCommentRepository(testDB),
		NewVerificationCodes(testCodes, &recordingMailer{}),
		testUsers,
	)

	email := "delete-revoke@example.com"
	user := serviceRegister(t, testAuth, email, "password123")

	export, err := account.Delete(testCtx, user.UID, seedCode(t, FlowDeletion, email))
	if err != nil {
		t.Fatalf("令牌清理失败不应让注销整体失败: %v", err)
	}
	if export.User.Email != email {
		t.Errorf("export email = %q, want %q（副本必须在匿名化之前构建）", export.User.Email, email)
	}

	anon, err := users.FindByUID(testCtx, user.UID)
	if err != nil {
		t.Fatalf("find anonymized user: %v", err)
	}
	if anon.Password != "" {
		t.Error("账号仍应完成匿名化")
	}
}

// TestAccountService_Export 导出应含资料与评论，且不含密码与操作日志
func TestAccountService_Export(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "mydata@example.com", "password123")

	commentService := testComments
	for i, content := range []string{"第一条", "第二条"} {
		if _, err := commentService.Create(testCtx, user.UID, &model.CreateCommentRequest{
			Content:    content,
			CommentKey: "ck1:export" + strconv.Itoa(i),
		}); err != nil {
			t.Fatalf("Failed to create comment: %v", err)
		}
	}

	export, err := testAccount.Export(testCtx, user.UID)
	if err != nil {
		t.Fatalf("Failed to export user data: %v", err)
	}

	if export.User.Email != "mydata@example.com" {
		t.Errorf("Expected email in export, got '%s'", export.User.Email)
	}
	if len(export.Comments) != 2 {
		t.Errorf("Expected 2 comments in export, got %d", len(export.Comments))
	}
	if export.ExportedAt.IsZero() {
		t.Error("Expected exported_at to be set")
	}
}
