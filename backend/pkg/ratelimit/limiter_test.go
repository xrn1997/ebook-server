package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter_Allow_WithinLimit(t *testing.T) {
	l := New(3, time.Minute)
	now := time.Now()

	if !l.AllowAt("key1", now) {
		t.Error("First request should be allowed")
	}
	if !l.AllowAt("key1", now.Add(1*time.Second)) {
		t.Error("Second request should be allowed")
	}
	if !l.AllowAt("key1", now.Add(2*time.Second)) {
		t.Error("Third request should be allowed")
	}
}

func TestLimiter_Allow_ExceedLimit(t *testing.T) {
	l := New(2, time.Minute)
	now := time.Now()

	l.AllowAt("key1", now)
	l.AllowAt("key1", now.Add(1*time.Second))

	if l.AllowAt("key1", now.Add(2*time.Second)) {
		t.Error("Third request should be rejected (limit=2)")
	}
}

func TestLimiter_Allow_WindowExpired(t *testing.T) {
	l := New(2, time.Minute)
	now := time.Now()

	// 达到上限
	l.AllowAt("key1", now)
	l.AllowAt("key1", now.Add(1*time.Second))
	if l.AllowAt("key1", now.Add(2*time.Second)) {
		t.Error("Should be rejected within window")
	}

	// 窗口过期后应重新允许
	if !l.AllowAt("key1", now.Add(61*time.Second)) {
		t.Error("Should be allowed after window expired")
	}
}

func TestLimiter_DifferentKeys(t *testing.T) {
	l := New(1, time.Minute)
	now := time.Now()

	if !l.AllowAt("key1", now) {
		t.Error("key1 first request should be allowed")
	}
	if l.AllowAt("key1", now.Add(1*time.Second)) {
		t.Error("key1 second request should be rejected")
	}
	// 不同 key 应独立计数
	if !l.AllowAt("key2", now.Add(1*time.Second)) {
		t.Error("key2 first request should be allowed")
	}
}

func TestLimiter_Allow_ConvenienceMethod(t *testing.T) {
	l := New(1, time.Minute)

	if !l.Allow("test") {
		t.Error("First Allow should succeed")
	}
	if l.Allow("test") {
		t.Error("Second Allow should fail")
	}
}

func TestLimiter_AllowAt_PartialWindowExpiry(t *testing.T) {
	l := New(2, time.Minute)
	now := time.Now()

	// 第一个请求在窗口起始
	l.AllowAt("key1", now)
	// 第二个请求在窗口中段
	l.AllowAt("key1", now.Add(30*time.Second))
	// 第三个请求被拒
	if l.AllowAt("key1", now.Add(31*time.Second)) {
		t.Error("Should be rejected (limit=2)")
	}

	// 第一个请求过期后（60秒后），应能再次请求
	if !l.AllowAt("key1", now.Add(61*time.Second)) {
		t.Error("Should be allowed after first hit expired")
	}
}

// TestLimiter_Peek 只判断不记录：Peek 本身不消耗配额，也不受后续 Allow 记录影响判定。
func TestLimiter_Peek(t *testing.T) {
	l := New(3, time.Minute)
	now := time.Now()

	// 空窗口不锁定
	if l.PeekAt("k", now) {
		t.Error("empty window should not be limited")
	}
	// Peek 不记录：连续 Peek 不改变状态
	if l.PeekAt("k", now.Add(1*time.Second)) {
		t.Error("Peek must not record hits")
	}

	// 2 次命中仍未达上限
	l.AllowAt("k", now.Add(2*time.Second))
	l.AllowAt("k", now.Add(3*time.Second))
	if l.PeekAt("k", now.Add(4*time.Second)) {
		t.Error("2 hits < limit 3 should not be limited")
	}

	// 第 3 次命中后锁定
	l.AllowAt("k", now.Add(5*time.Second))
	if !l.PeekAt("k", now.Add(6*time.Second)) {
		t.Error("3 hits should be limited")
	}

	// 窗口过期后解除
	if l.PeekAt("k", now.Add(2*time.Minute)) {
		t.Error("window expired should reset")
	}
}

// TestAllowAll_AllWindowsPassRecordsEveryWindow 全过才记：一次成功在每个窗口各记一次。
func TestAllowAll_AllWindowsPassRecordsEveryWindow(t *testing.T) {
	minute, hour := New(2, time.Minute), New(2, time.Hour)

	if !AllowAll([]*Limiter{minute, hour}, "k") {
		t.Fatal("both windows under limit should allow")
	}
	// 两个窗口都被记了一次：第二次仍应全过，第三次起两窗一起拒
	if !AllowAll([]*Limiter{minute, hour}, "k") {
		t.Error("second call should still pass (limit 2 on each window)")
	}
	if AllowAll([]*Limiter{minute, hour}, "k") {
		t.Error("third call should be rejected (limit 2 on each window)")
	}
	if !minute.PeekAt("k", time.Now()) || !hour.PeekAt("k", time.Now()) {
		t.Error("a successful AllowAll must record every window")
	}
}

// TestAllowAll_RejectionRecordsNoWindow 中途拒不记任何窗口：
// 被 hour 拒掉的尝试不消耗 minute 配额（短路逐个 Allow 会白扣一次）。
func TestAllowAll_RejectionRecordsNoWindow(t *testing.T) {
	minute, hour := New(1, time.Minute), New(1, time.Hour)
	hour.AllowAt("k", time.Now()) // hour 已满，minute 仍空

	if AllowAll([]*Limiter{minute, hour}, "k") {
		t.Fatal("exhausted hour window should reject")
	}
	if minute.PeekAt("k", time.Now()) {
		t.Error("rejected attempt must not record the minute window")
	}
}

// TestAllowAll_SingleLimiterMatchesAllow 单窗口退化形态与 Allow 同语义。
func TestAllowAll_SingleLimiterMatchesAllow(t *testing.T) {
	l := New(1, time.Minute)

	if !AllowAll([]*Limiter{l}, "k") {
		t.Fatal("first call should be allowed")
	}
	if AllowAll([]*Limiter{l}, "k") {
		t.Error("second call should be rejected (limit 1)")
	}
	if !l.PeekAt("k", time.Now()) {
		t.Error("AllowAll on one limiter should record it like Allow")
	}
}
