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
