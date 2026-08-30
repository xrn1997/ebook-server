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
// 以调用方传入的 key 为键（形如 "reg:"+email / "forgot:"+email，见 service/auth.go），
// 支持验证码再获取与错误重试次数限制（A0241）。
type Store struct {
	mu    sync.RWMutex
	codes map[string]*item
}

// NewStore 创建验证码存储实例。
//
// 每个实例独立持有状态：生产由 main.go 创建唯一实例并注入各 service，
// 测试各建各的——历史上本包曾通过 Default() 暴露全局默认存储（已删除，ADR-0007）。
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

// Save 以 key 为键保存新生成的验证码并返回该验证码
func (s *Store) Save(key string) string {
	codeVal := NewCode()
	s.Set(key, codeVal)
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
func (s *Store) Verify(key, codeVal string) VerifyResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	it, ok := s.codes[key]
	if !ok || time.Now().After(it.expireAt) {
		return ResultNotFound
	}
	if it.code != codeVal {
		it.attempts++
		if it.attempts >= MaxAttempts {
			delete(s.codes, key)
			return ResultTooManyAttempts
		}
		return ResultWrong
	}

	delete(s.codes, key)
	return ResultOK
}
