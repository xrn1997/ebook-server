package code

import "testing"

func TestStore_Verify_Success(t *testing.T) {
	s := NewStore()
	codeVal := s.Save("user")
	if got := s.Verify("user", codeVal); got != ResultOK {
		t.Fatalf("Expected ResultOK, got %v", got)
	}
	// 已验证码被删除，再次校验应无有效验证码
	if got := s.Verify("user", codeVal); got != ResultNotFound {
		t.Fatalf("Expected ResultNotFound after use, got %v", got)
	}
}

func TestStore_Verify_NotFound(t *testing.T) {
	s := NewStore()
	if got := s.Verify("nobody", "123456"); got != ResultNotFound {
		t.Fatalf("Expected ResultNotFound, got %v", got)
	}
}

func TestStore_Verify_WrongThenTooMany(t *testing.T) {
	s := NewStore()
	_ = s.Save("user")

	// 前 4 次错误
	for i := 0; i < MaxAttempts-1; i++ {
		if got := s.Verify("user", "000000"); got != ResultWrong {
			t.Fatalf("attempt %d: expected ResultWrong, got %v", i+1, got)
		}
	}
	// 第 5 次错误触发超限
	if got := s.Verify("user", "000000"); got != ResultTooManyAttempts {
		t.Fatalf("Expected ResultTooManyAttempts, got %v", got)
	}
}

func TestStore_NewCode_IsSixDigits(t *testing.T) {
	for i := 0; i < 100; i++ {
		c := NewCode()
		if len(c) != 6 {
			t.Fatalf("Expected 6-digit code, got %q", c)
		}
	}
}
