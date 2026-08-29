package model

import (
	"testing"
	"time"
)

func TestUserTableName(t *testing.T) {
	user := User{}
	if user.TableName() != "users" {
		t.Errorf("Expected 'users', got '%s'", user.TableName())
	}
}

func TestCommentTableName(t *testing.T) {
	comment := Comment{}
	if comment.TableName() != "comments" {
		t.Errorf("Expected 'comments', got '%s'", comment.TableName())
	}
}

func TestOperationLogTableName(t *testing.T) {
	log := OperationLog{}
	if log.TableName() != "operation_logs" {
		t.Errorf("Expected 'operation_logs', got '%s'", log.TableName())
	}
}

func TestUserFields(t *testing.T) {
	user := User{
		UID:      1,
		Email:    "test@example.com",
		Username: "testuser",
		Avatar:   "https://example.com/avatar.jpg",
	}

	if user.UID != 1 {
		t.Errorf("Expected UID 1, got %d", user.UID)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", user.Email)
	}
	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}
}

func TestCommentFields(t *testing.T) {
	comment := Comment{
		ID:      1,
		UserID:  1,
		Content: "This is a comment",
	}

	if comment.ID != 1 {
		t.Errorf("Expected ID 1, got %d", comment.ID)
	}
	if comment.UserID != 1 {
		t.Errorf("Expected UserID 1, got %d", comment.UserID)
	}
	if comment.Content != "This is a comment" {
		t.Errorf("Expected Content 'This is a comment', got '%s'", comment.Content)
	}
}

func TestOperationLogFields(t *testing.T) {
	log := OperationLog{
		ID:           1,
		UserID:       1,
		Username:     "testuser",
		Method:       "POST",
		Path:         "/api/comments",
		IP:           "127.0.0.1",
		ResponseCode: 200,
		CreatedAt:    time.Now(),
	}

	if log.Method != "POST" {
		t.Errorf("Expected Method 'POST', got '%s'", log.Method)
	}
	if log.Path != "/api/comments" {
		t.Errorf("Expected Path '/api/comments', got '%s'", log.Path)
	}
	if log.ResponseCode != 200 {
		t.Errorf("Expected ResponseCode 200, got %d", log.ResponseCode)
	}
}

func TestRegisterRequestFields(t *testing.T) {
	req := RegisterRequest{
		Email:    "test@example.com",
		Code:     "123456",
		Password: "123456",
	}

	// 校验结构体字段承载（gin binding 标签在 handler 层触发，这里仅确认结构存在）
	if req.Email == "" {
		t.Error("Email should not be empty")
	}
	if len(req.Code) != 6 {
		t.Errorf("Code should be 6-digit, got %q", req.Code)
	}
	if req.Password == "" {
		t.Error("Password should not be empty")
	}
}

func TestCommentListResponse(t *testing.T) {
	response := CommentListResponse{
		Items:    []Comment{},
		Total:    0,
		Page:     1,
		PageSize: 10,
	}

	if response.Page != 1 {
		t.Errorf("Expected Page 1, got %d", response.Page)
	}
	if response.PageSize != 10 {
		t.Errorf("Expected PageSize 10, got %d", response.PageSize)
	}
}

func TestLogListResponse(t *testing.T) {
	response := LogListResponse{
		Items:    []OperationLog{},
		Total:    0,
		Page:     1,
		PageSize: 10,
	}

	if response.Page != 1 {
		t.Errorf("Expected Page 1, got %d", response.Page)
	}
	if response.PageSize != 10 {
		t.Errorf("Expected PageSize 10, got %d", response.PageSize)
	}
}
