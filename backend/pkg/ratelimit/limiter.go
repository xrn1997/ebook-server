// Package ratelimit 提供内存固定窗口限流器，用于发码防轰炸等轻量场景。
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

	l.hits[key] = trimHits(l.hits[key], now, l.window)
	if len(l.hits[key]) >= l.limit {
		return false
	}
	l.hits[key] = append(l.hits[key], now)
	return true
}

// Peek 判断 key 在窗口内是否已达上限，**不记录**本次访问。
//
// 配合「先检查后记录」的语义使用：登录限流只在密码校验失败时记录失败次数，
// 请求到达时先 Peek 判断是否已锁定——成功登录因此不会被计入配额。
func (l *Limiter) Peek(key string) bool {
	return l.PeekAt(key, time.Now())
}

// PeekAt 便于测试指定时刻
func (l *Limiter) PeekAt(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = trimHits(l.hits[key], now, l.window)
	return len(l.hits[key]) >= l.limit
}

// trimHits 清理窗口外的命中记录，返回保留在窗口内的部分。
func trimHits(hits []time.Time, now time.Time, window time.Duration) []time.Time {
	old := now.Add(-window)
	kept := hits[:0]
	for _, t := range hits {
		if t.After(old) {
			kept = append(kept, t)
		}
	}
	return kept
}
