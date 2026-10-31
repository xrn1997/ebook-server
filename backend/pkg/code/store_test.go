package code

import (
	"testing"
	"time"
)

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

// TestStore_Verify_Expired 过期的码与「从来没有码」是同一答案：不得再被消费。
//
// 本包不注入时钟（TTL 直接取 time.Now），所以把 expireAt 写成过去时刻来命中这条分支，
// 而不是 sleep 满 TTL。
func TestStore_Verify_Expired(t *testing.T) {
	s := NewStore()
	s.codes["reg:user@example.com"] = &item{
		code:     "123456",
		expireAt: time.Now().Add(-time.Second),
	}

	if got := s.Verify("reg:user@example.com", "123456"); got != ResultNotFound {
		t.Errorf("Expected ResultNotFound after TTL, got %v", got)
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
