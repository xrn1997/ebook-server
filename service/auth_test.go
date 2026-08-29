package service

import (
	"ebook-server/config"
	"ebook-server/model"
	"ebook-server/pkg/database"
	"testing"
	"time"

	"gorm.io/gorm"
)

var testDB *gorm.DB

func setupTestDB(t *testing.T) {
	t.Helper()

	// 重置数据库连接
	if database.DB != nil {
		sqlDB, _ := database.DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		database.DB = nil
	}

	// 配置测试数据库（使用内存数据库）
	config.AppConfig = &config.Config{
		JWT: config.JWTConfig{
			Secret:     "test-secret",
			ExpireHour: 1,
		},
		Database: config.DatabaseConfig{
			Path: ":memory:",
		},
	}

	// 初始化数据库
	if err := database.Init(); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}

	testDB = database.GetDB()

	// 自动迁移
	testDB.AutoMigrate(&model.User{}, &model.Comment{}, &model.OperationLog{})
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

func TestAuthService_Register_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()

	req := &model.RegisterRequest{
		Username: "testuser_" + string(rune(time.Now().UnixNano()%10000)),
		Password: "password123",
		Email:    "test@example.com",
	}

	user, err := authService.Register(req)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	if user.Username != req.Username {
		t.Errorf("Expected username '%s', got '%s'", req.Username, user.Username)
	}

	if user.Email != req.Email {
		t.Errorf("Expected email '%s', got '%s'", req.Email, user.Email)
	}

	// Password should be hashed
	if user.Password == req.Password {
		t.Error("Password should be hashed, not plain text")
	}
}

func TestAuthService_Register_DuplicateUsername(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()

	req := &model.RegisterRequest{
		Username: "duplicate_user",
		Password: "password123",
	}

	// First registration
	_, err := authService.Register(req)
	if err != nil {
		t.Fatalf("Failed to register first user: %v", err)
	}

	// Second registration with same username
	_, err = authService.Register(req)
	if err != model.ErrUsernameExists {
		t.Errorf("Expected ErrUsernameExists, got %v", err)
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()

	// Register first
	registerReq := &model.RegisterRequest{
		Username: "loginuser",
		Password: "password123",
	}
	_, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	// Login
	loginReq := &model.LoginRequest{
		Username: "loginuser",
		Password: "password123",
	}

	token, user, err := authService.Login(loginReq)
	if err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	if token == "" {
		t.Error("Token should not be empty")
	}

	if user.Username != loginReq.Username {
		t.Errorf("Expected username '%s', got '%s'", loginReq.Username, user.Username)
	}
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()

	loginReq := &model.LoginRequest{
		Username: "nonexistent",
		Password: "wrongpassword",
	}

	_, _, err := authService.Login(loginReq)
	if err != model.ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	authService := NewAuthService()

	// Register first
	registerReq := &model.RegisterRequest{
		Username: "wrongpassuser",
		Password: "correctpassword",
	}
	_, err := authService.Register(registerReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	// Login with wrong password
	loginReq := &model.LoginRequest{
		Username: "wrongpassuser",
		Password: "wrongpassword",
	}

	_, _, err = authService.Login(loginReq)
	if err != model.ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}
}
