package handler

import (
	"ebook-server/config"
	"ebook-server/model"
	"ebook-server/pkg/jwt"
	"time"
)

// TestConfig 测试配置
var TestConfig = &config.Config{
	Server: config.ServerConfig{
		Port: 8080,
		Mode: "test",
	},
	Database: config.DatabaseConfig{
		Path: "test.db",
	},
	JWT: config.JWTConfig{
		Secret:     "test-secret-key",
		ExpireHour: 1,
	},
}

// SetupTestConfig 初始化测试配置
func SetupTestConfig() {
	config.AppConfig = TestConfig
}

// GenerateTestToken 生成测试用 Token
func GenerateTestToken(userID uint, username string) (string, error) {
	return jwt.GenerateToken(userID, username)
}

// CreateTestUser 创建测试用户
func CreateTestUser() *model.User {
	return &model.User{
		ID:       1,
		Username: "testuser",
		Email:    "test@example.com",
		Avatar:   "https://example.com/avatar.jpg",
	}
}

// CreateTestComment 创建测试评论
func CreateTestComment(userID uint) *model.Comment {
	return &model.Comment{
		ID:        1,
		UserID:    userID,
		Content:   "This is a test comment",
		CreatedAt: time.Now(),
	}
}

// CreateTestLog 创建测试日志
func CreateTestLog(userID uint) *model.OperationLog {
	return &model.OperationLog{
		ID:           1,
		UserID:       userID,
		Username:     "testuser",
		Method:       "GET",
		Path:         "/api/test",
		IP:           "127.0.0.1",
		ResponseCode: 200,
		CreatedAt:    time.Now(),
	}
}
