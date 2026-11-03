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

// 多窗口组合用包级函数表达：Limiter 是唯一具体类型，不引入接口或包装类型
// （ADR-0007 §6 的具体类型原则）。

// AllowAll 在多个窗口上「要么都记、要么都不记」地记账一次。
//
// 语义：先查全部窗口，全部未达上限才在每个窗口各记一次并返回 true；任一窗口已达
// 上限则拒绝，且不记任何窗口。这是多窗口记账的唯一入口——短路写法（`!a.Allow(k) ||
// !b.Allow(k)`）会先记满 a 再撞 b：一次被 b 拒绝的尝试白白消耗 a 的配额，下一窗口
// 恢复前连带锁死 a 的余量。发码双窗口限流（service.VerificationCodes.Send）经此处；
// 单窗口调用方直接用 Limiter 的 Peek/Allow（后台登录即「先 Peek 问锁、失败才 Allow
// 记账」的单窗口形态），不构造单元素切片。
//
// 本包不另配一个不记账的 PeekAll：多窗口面目前只有「记账」这一个调用方，先问锁的
// 形态都是单窗口。等真出现第二个调用方再补，不要留一份没人用的对称 API。
//
// 「查全 → 记全」不是跨窗口事务：每个 Limiter 各持一把锁，并发交错下仍可能出现
// 「查时未满、记时已满」——此时返回 false，先前已记的窗口不回滚。限流是防滥用的
// 兜底而非计费，单进程内存实现下这点交错可接受；需要严格原子时由调用方自带临界区
// （后台登录的 loginMu 即是）。
func AllowAll(limiters []*Limiter, key string) bool {
	for _, l := range limiters {
		if l.Peek(key) {
			return false
		}
	}
	for _, l := range limiters {
		if !l.Allow(key) {
			return false
		}
	}
	return true
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
