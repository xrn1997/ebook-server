package service

import (
	"ebook-server/config"
	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/database"
	"testing"

	"gorm.io/gorm"
)

var testDB *gorm.DB

// serviceTestCode 测试注册码
const serviceTestCode = "123456"

func setupTestDB(t *testing.T) {
	t.Helper()

	if database.DB != nil {
		sqlDB, _ := database.DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		database.DB = nil
	}

	config.AppConfig = &config.Config{
		Server: config.ServerConfig{Mode: "test"},
		JWT: config.JWTConfig{
			Secret:    "test-secret",
			ExpireMin: 1,
		},
		Database: config.DatabaseConfig{
			Path: ":memory:",
		},
	}

	if err := database.Init(); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}

	testDB = database.GetDB()
	testDB.AutoMigrate(&model.User{}, &model.Comment{}, &model.OperationLog{}, &model.RefreshToken{})
}

func cleanupTestDB(t *testing.T) {
	t.Helper()
	if database.DB != nil {
		sqlDB, _ := database.DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		database.DB = nil
	}
}

// serviceRegister 注册一个用户（注入固定验证码）
func serviceRegister(t *testing.T, s *AuthService, email, password string) *model.User {
	t.Helper()
	code.Default().Set("reg:"+email, serviceTestCode)
	user, err := s.Register(&model.RegisterRequest{
		Email:    email,
		Code:     serviceTestCode,
		Password: password,
	})
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}
	return user
}

func TestAuthService_Register_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	email := "svc-reg@example.com"
	user := serviceRegister(t, authService, email, "password123")

	if user.Email != email {
		t.Errorf("Expected email %q, got %q", email, user.Email)
	}
	if user.UID == 0 {
		t.Error("Expected non-zero uid")
	}
	if user.Username == "" {
		t.Error("Expected auto-generated username")
	}
	if user.Password == "password123" {
		t.Error("Password should be hashed")
	}
}

func TestAuthService_Register_WrongCode(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	email := "svc-reg-wrong@example.com"
	code.Default().Set("reg:"+email, "000000")

	_, err := authService.Register(&model.RegisterRequest{Email: email, Code: "111111", Password: "password123"})
	if err != model.ErrCodeInvalid {
		t.Errorf("Expected ErrCodeInvalid, got %v", err)
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	email := "svc-reg-dup@example.com"
	serviceRegister(t, authService, email, "password123")

	_, err := authService.Register(&model.RegisterRequest{Email: email, Code: "654321", Password: "password123"})
	if err != model.ErrEmailExists {
		t.Errorf("Expected ErrEmailExists, got %v", err)
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	email := "svc-login@example.com"
	serviceRegister(t, authService, email, "password123")

	pair, err := authService.Login(&model.LoginRequest{Email: email, Password: "password123"})
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if pair.RefreshToken == "" {
		t.Error("RefreshToken should not be empty")
	}
}

func TestAuthService_Login_AccountNotFound(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	_, err := authService.Login(&model.LoginRequest{Email: "ghost@example.com", Password: "x"})
	if err != model.ErrAccountNotFound {
		t.Errorf("Expected ErrAccountNotFound, got %v", err)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	email := "svc-login-wrong@example.com"
	serviceRegister(t, authService, email, "password123")

	_, err := authService.Login(&model.LoginRequest{Email: email, Password: "wrong"})
	if err != model.ErrPasswordWrong {
		t.Errorf("Expected ErrPasswordWrong, got %v", err)
	}
}

func TestAuthService_Login_Lockout(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	email := "svc-lock@example.com"
	serviceRegister(t, authService, email, "password123")

	// 连续 5 次错误密码：第 5 次触发锁定
	for i := 0; i < 5; i++ {
		_, err := authService.Login(&model.LoginRequest{Email: email, Password: "wrong"})
		if i < 4 {
			if err != model.ErrPasswordWrong {
				t.Fatalf("attempt %d expected ErrPasswordWrong, got %v", i+1, err)
			}
		} else {
			if err != model.ErrAttemptTooMany {
				t.Fatalf("expected ErrAttemptTooMany on 5th failure, got %v", err)
			}
		}
	}

	// 锁定期间正确密码也被拒绝
	_, err := authService.Login(&model.LoginRequest{Email: email, Password: "password123"})
	if err != model.ErrAccountLocked {
		t.Errorf("Expected ErrAccountLocked while locked, got %v", err)
	}
}

func TestAuthService_Refresh_Rotation(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	email := "svc-refresh@example.com"
	serviceRegister(t, authService, email, "password123")

	pair, err := authService.Login(&model.LoginRequest{Email: email, Password: "password123"})
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	newPair, err := authService.Refresh(pair.RefreshToken)
	if err != nil {
		t.Fatalf("Failed to refresh: %v", err)
	}
	if newPair.RefreshToken == pair.RefreshToken {
		t.Error("Refresh token should be rotated")
	}

	_, err = authService.Refresh(pair.RefreshToken)
	if err != model.ErrLoginExpired {
		t.Errorf("Expected ErrLoginExpired after rotation, got %v", err)
	}
}

func TestAuthService_ChangePassword_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	email := "svc-changepwd@example.com"
	user := serviceRegister(t, authService, email, "password123")

	if err := authService.ChangePassword(user.UID, "password123", "newpass456"); err != nil {
		t.Fatalf("Failed to change password: %v", err)
	}

	_, err := authService.Login(&model.LoginRequest{Email: email, Password: "newpass456"})
	if err != nil {
		t.Errorf("Expected login with new password to succeed, got %v", err)
	}
}

func TestAuthService_ResetPassword_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()
	email := "svc-forgot@example.com"
	serviceRegister(t, authService, email, "password123")

	// 注入找回密码验证码（key 为 forgot:<email>）
	code.Default().Set("forgot:"+email, serviceTestCode)
	if err := authService.ResetPassword(email, serviceTestCode, "newpass456"); err != nil {
		t.Fatalf("Failed to reset password: %v", err)
	}

	_, err := authService.Login(&model.LoginRequest{Email: email, Password: "newpass456"})
	if err != nil {
		t.Errorf("Expected login with new password after reset, got %v", err)
	}
}
