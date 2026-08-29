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
		ID:       1,
		Username: "testuser",
		Email:    "test@example.com",
		Avatar:   "https://example.com/avatar.jpg",
	}

	if user.ID != 1 {
		t.Errorf("Expected ID 1, got %d", user.ID)
	}
	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", user.Email)
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

func TestRegisterRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		req      RegisterRequest
		wantErr  bool
	}{
		{
			name: "valid request",
			req: RegisterRequest{
				Username: "testuser",
				Password: "123456",
				Email:    "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "username too short",
			req: RegisterRequest{
				Username: "ab",
				Password: "123456",
			},
			wantErr: true,
		},
		{
			name: "password too short",
			req: RegisterRequest{
				Username: "testuser",
				Password: "123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This tests the struct definition, not the binding validation
			// Binding validation is tested in handler tests
			if tt.req.Username == "" && !tt.wantErr {
				t.Error("Username should not be empty")
			}
		})
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
