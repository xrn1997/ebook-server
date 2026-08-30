package service

import (
	"strconv"
	"testing"

	"ebook-server/model"
	"ebook-server/pkg/code"
)

// TestAuthService_SendDeletionCode 注销验证码按独立命名空间存储与限流（ADR-0005）
func TestAuthService_SendDeletionCode(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "del-code@example.com", "password123")

	if err := testAccount.SendDeletionCode(user.UID); err != nil {
		t.Fatalf("Failed to send deletion code: %v", err)
	}

	// 应落在 del: 命名空间，与注册/找回密码隔离
	if got := testCodes.Verify("del:del-code@example.com", ""); got == code.ResultNotFound {
		t.Error("Expected a deletion code stored under 'del:' namespace")
	}
}

// TestAuthService_DeleteAccount_Anonymize 注销后账号被匿名化（ADR-0005）
func TestAuthService_DeleteAccount_Anonymize(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "anon@example.com", "password123")

	testCodes.Set("del:anon@example.com", serviceTestCode)
	export, err := testAccount.Delete(user.UID, serviceTestCode)
	if err != nil {
		t.Fatalf("Failed to delete account: %v", err)
	}

	// 导出应保留注销前的真实资料，而不是占位值
	if export.User.Email != "anon@example.com" {
		t.Errorf("Expected exported email 'anon@example.com', got '%s'", export.User.Email)
	}

	anon, err := testAccount.users.FindByUID(user.UID)
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

	testCodes.Set("del:reuse@example.com", serviceTestCode)
	if _, err := testAccount.Delete(user.UID, serviceTestCode); err != nil {
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
	pair, err := authService.Login(&model.LoginRequest{Email: email, Password: password})
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
	if _, err := authService.Refresh(before.RefreshToken); err != nil {
		t.Fatalf("Refresh should succeed before deletion: %v", err)
	}

	testCodes.Set("del:revoke@example.com", serviceTestCode)
	if _, err := testAccount.Delete(user.UID, serviceTestCode); err != nil {
		t.Fatalf("Failed to delete account: %v", err)
	}

	if _, err := authService.Refresh(after.RefreshToken); err != model.ErrLoginExpired {
		t.Errorf("Refresh should fail with ErrLoginExpired after deletion, got: %v", err)
	}
}

// TestAuthService_DeleteAccount_WrongCode 验证码错误时应拒绝注销，账号保持原样
func TestAuthService_DeleteAccount_WrongCode(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "wrongcode@example.com", "password123")

	testCodes.Set("del:wrongcode@example.com", serviceTestCode)
	if _, err := testAccount.Delete(user.UID, "000000"); err != model.ErrCodeInvalid {
		t.Errorf("Expected ErrCodeInvalid, got %v", err)
	}

	got, err := testAccount.users.FindByUID(user.UID)
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
	if _, err := commentService.Create(user.UID, &model.CreateCommentRequest{
		Content: "我的一条评论",
	}); err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	testCodes.Set("del:export@example.com", serviceTestCode)
	export, err := testAccount.Delete(user.UID, serviceTestCode)
	if err != nil {
		t.Fatalf("Failed to delete account: %v", err)
	}

	if len(export.Comments) != 1 {
		t.Fatalf("Expected 1 exported comment, got %d", len(export.Comments))
	}
	if export.Comments[0].Content != "我的一条评论" {
		t.Errorf("Expected exported comment content, got '%s'", export.Comments[0].Content)
	}
}

// TestAccountService_Export 导出应含资料与评论，且不含密码与操作日志
func TestAccountService_Export(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := testAuth
	user := serviceRegister(t, authService, "mydata@example.com", "password123")

	commentService := testComments
	for _, content := range []string{"第一条", "第二条"} {
		if _, err := commentService.Create(user.UID, &model.CreateCommentRequest{Content: content}); err != nil {
			t.Fatalf("Failed to create comment: %v", err)
		}
	}

	export, err := testAccount.Export(user.UID)
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
