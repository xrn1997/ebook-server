// Package code 提供内存验证码存储与校验，支持 TTL 过期与重试次数限制。
package code

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
)

const (
	// TTL 验证码有效期
	TTL = 5 * time.Minute
	// MaxAttempts 校验失败最大次数
	MaxAttempts = 5
)

// VerifyResult 验证码校验结果
type VerifyResult int

const (
	// ResultOK 校验通过
	ResultOK VerifyResult = iota
	// ResultNotFound 无有效验证码（未生成或已过期）
	ResultNotFound
	// ResultWrong 验证码错误（已计数）
	ResultWrong
	// ResultTooManyAttempts 尝试次数超限
	ResultTooManyAttempts
)

// item 验证码内存记录
type item struct {
	code     string
	expireAt time.Time
	attempts int
}

// Store 基于内存 map 的验证码存储（单进程直接可用）
//
// 以 username 为键，支持验证码再获取与错误重试次数限制（A0241）。
type Store struct {
	mu    sync.RWMutex
	codes map[string]*item
}

var defaultStore = &Store{codes: make(map[string]*item)}

// NewStore 创建独立验证码存储（测试可用）
func NewStore() *Store {
	return &Store{codes: make(map[string]*item)}
}

// NewCode 生成 6 位数字验证码
func NewCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "000000"
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// Save 保存验证码并返回生成的验证码
func (s *Store) Save(username string) string {
	codeVal := NewCode()
	s.Set(username, codeVal)
	return codeVal
}

// Set 直接写入指定验证码（测试或重放场景使用）
func (s *Store) Set(key, codeVal string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[key] = &item{
		code:     codeVal,
		expireAt: time.Now().Add(TTL),
	}
}

// Verify 校验验证码：成功删除记录并返回 ResultOK；
// 无有效验证码返回 ResultNotFound；验证码错误计数并返回 ResultWrong，
// 达到最大次数返回 ResultTooManyAttempts。
func (s *Store) Verify(username, codeVal string) VerifyResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	it, ok := s.codes[username]
	if !ok || time.Now().After(it.expireAt) {
		return ResultNotFound
	}
	if it.code != codeVal {
		it.attempts++
		if it.attempts >= MaxAttempts {
			delete(s.codes, username)
			return ResultTooManyAttempts
		}
		return ResultWrong
	}

	delete(s.codes, username)
	return ResultOK
}

// Default 返回全局默认验证码存储
func Default() *Store {
	return defaultStore
}
