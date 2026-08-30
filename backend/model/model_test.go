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

func TestRefreshTokenTableName(t *testing.T) {
	token := RefreshToken{}
	if token.TableName() != "refresh_tokens" {
		t.Errorf("Expected 'refresh_tokens', got '%s'", token.TableName())
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
		Items:    []CommentResponse{},
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

// TestNewCommentResponse 评论响应视图契约（ADR-0011）：
// user 只含 4 个展示字段（不含 email），add_time 固定上海时区格式。
func TestNewCommentResponse(t *testing.T) {
	created := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
	c := &Comment{
		ID:          7,
		UserID:      3,
		User:        User{UID: 3, Email: "leak@example.com", Username: "user03", Nickname: "昵称", Avatar: "http://x/a.png"},
		Content:     "好文",
		ChapterURL:  "https://src.example.com/book/1/2.html",
		ChapterName: "第三章",
		BookName:    "天启之书",
		CreatedAt:   created,
	}
	resp := NewCommentResponse(c)

	if resp.ID != 7 || resp.Content != "好文" {
		t.Errorf("unexpected base fields: %+v", resp)
	}
	if resp.ChapterURL != c.ChapterURL || resp.ChapterName != "第三章" || resp.BookName != "天启之书" {
		t.Errorf("chapter fields not carried: %+v", resp)
	}
	// user 视图四字段 + 不泄露 email
	if resp.User.UID != 3 || resp.User.Username != "user03" || resp.User.Nickname != "昵称" || resp.User.Avatar != "http://x/a.png" {
		t.Errorf("user view mismatch: %+v", resp.User)
	}
	// add_time：UTC 20:00 → 上海 +8 → 次日 04:00
	if resp.AddTime != "2026-08-31 04:00:00" {
		t.Errorf("add_time = %q, want 2026-08-31 04:00:00", resp.AddTime)
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
