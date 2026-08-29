package ratelimit

import (
	"sync"
	"time"
)

// Limiter 简单的固定内存限流器（单进程内有效）。
//
// 同一 key 在 window 内最多允许 limit 次；超出返回 false。用于发送验证码等
// 需要防轰炸的轻量场景，无需持久化。
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

// New 创建限流器：window 时间窗口内最多 limit 次
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:  limit,
		window: window,
		hits:   make(map[string][]time.Time),
	}
}

// Allow 记录一次访问并判断是否允许
func (l *Limiter) Allow(key string) bool {
	return l.AllowAt(key, time.Now())
}

// AllowAt 便于测试指定时刻
func (l *Limiter) AllowAt(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	old := now.Add(-l.window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(old) {
			kept = append(kept, t)
		}
	}
	l.hits[key] = kept

	if len(kept) >= l.limit {
		return false
	}
	l.hits[key] = append(l.hits[key], now)
	return true
}
