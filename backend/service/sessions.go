package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"ebook-server/model"
)

// RefreshTokenTTL refresh token 有效期（30 天）
const RefreshTokenTTL = 30 * 24 * time.Hour

// refreshTokenStore 会话模块自持的 refresh token 存储能力（模块私有，不进 ports.go）。
//
// 与 ports.go 的 consumer-defined 接口同一套道理，但这份能力只服务 Sessions 自己：
// 它不是给别的模块的接缝，而是把「一次会话」的数据面收进模块边界内。
// Issue/Consume/Revoke 的数据面是这一组：签发（Create）、按哈希查未过期行（FindByHash）、
// 就地轮换（Rotate，条件更新而不是删+插）、按用户撤销（DeleteByUserID）。
// 过期清理不在这一组语义里，也不进这个接口：FindByHash 只认未过期的行，过期记录因此
// 行为惰性，回收的只是磁盘；repository 留着 DeleteExpired 当预埋的清理原语，等定时清理
// 任务真落地时再扩接口（口径见 docs/development.md「当前已知问题」）。
type refreshTokenStore interface {
	Create(ctx context.Context, token *model.RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*model.RefreshToken, error)
	// Rotate 就地轮换并返回是否改到行：轮换的「单次消费」「撤销优先」与「过期优先」
	// 判据都在这一条条件更新里（只有仍持有 oldHash 且未到期的一方换得到）。
	Rotate(ctx context.Context, id uint, oldHash, newHash string, expiresAt time.Time) (bool, error)
	DeleteByUserID(ctx context.Context, userID uint) error
}

// sessionUserStore 会话模块需要的账号读取能力（模块私有，不进 ports.go）。
//
// 只有 Consume 的持有者状态检查一处用到它：接口宽度由消费方决定，不为一读依赖
// 整份账号读面（ADR-0007，与 CredentialStore.FindByUID 同一套道理）。
type sessionUserStore interface {
	FindByUID(ctx context.Context, uid uint) (*model.User, error)
}

// Sessions 一次会话的完整生命周期：签发（Issue）、消费（Consume）、撤销（Revoke）。
//
// refresh token 的规则全部住在这里（ADR-0003）：随机串只以 SHA-256 哈希落库、
// 轮换即单次消费、撤销按用户整批作废。access token 是无状态的，不经本模块，
// 也就撤不掉——它活到自身过期（ADR-0003 的取舍）。
type Sessions struct {
	tokens refreshTokenStore
	// users 承载 Consume 的匿名化状态检查：令牌指向的账号若已注销，它不得再续期，
	// 否则一条会话能在账号注销后无限期存活。这道检查与撤销、轮换的先后无关，
	// 是对库里既存令牌行的兜底。
	users sessionUserStore
}

// NewSessions 创建会话模块实例：两样依赖都是模块自持的私有接缝（ADR-0007）。
func NewSessions(tokens refreshTokenStore, users sessionUserStore) *Sessions {
	return &Sessions{tokens: tokens, users: users}
}

// Issue 签发一枚 refresh token：32 字节随机 hex 为 raw，只落库其 SHA-256 哈希，
// 有效期 RefreshTokenTTL。raw 仅返回这一次，服务端不再持有可复原的原文。
func (s *Sessions) Issue(ctx context.Context, uid uint) (string, error) {
	raw, err := newTokenRaw()
	if err != nil {
		return "", err
	}
	record := &model.RefreshToken{
		TokenHash: sha256Hex(raw),
		UserID:    uid,
		ExpiresAt: time.Now().Add(RefreshTokenTTL),
	}
	if err := s.tokens.Create(ctx, record); err != nil {
		return "", err
	}
	return raw, nil
}

// Consume 单次消费一枚 refresh token：验明持有者仍可用后**就地**换出新令牌，
// 返回持有者账号与新令牌原文。
//
// 顺序刻意是「先查状态、再轮换」：注销与刷新的竞态里，这道状态检查保证已匿名化
// 账号的令牌一律续不了期，与撤销的先后无关（isAnonymized）。轮换本身是一条打在
// 旧哈希上的条件更新——新令牌不与旧行分家，因此既没有并发双花的窗口，也没有
// 「撤销之后落库一枚新令牌」的漏网活口。
//
// 不回新令牌原文的失败一律是「请重新登录」：未找到/已过期、CAS 打不中（并发刷新
// 的败者，或撤销已把行删掉）对调用方是同一件事，共用 model.ErrLoginExpired
// （repository 的哨兵语义原样上浮）。
func (s *Sessions) Consume(ctx context.Context, raw string) (*model.User, string, error) {
	oldHash := sha256Hex(raw)
	record, err := s.tokens.FindByHash(ctx, oldHash)
	if err != nil {
		return nil, "", err // 令牌不存在或已过期，repository 一并给 model.ErrLoginExpired
	}

	user, err := s.users.FindByUID(ctx, record.UserID)
	if err != nil {
		return nil, "", err // 令牌指向的账号已被硬删（注销只匿名化，见 ADR-0005）→ A0201
	}
	if isAnonymized(user) {
		return nil, "", model.ErrAccountNotFound // 匿名化账号一律拒绝续期（→ A0201）
	}

	newRaw, err := newTokenRaw()
	if err != nil {
		return nil, "", err
	}
	rotated, err := s.tokens.Rotate(ctx, record.ID, oldHash, sha256Hex(newRaw), time.Now().Add(RefreshTokenTTL))
	if err != nil {
		return nil, "", err
	}
	if !rotated {
		return nil, "", model.ErrLoginExpired
	}
	return user, newRaw, nil
}

// Revoke 撤销一个用户的全部 refresh token（登出、改密、注销共用这一处）。
//
// access token 是无状态的，这里撤不掉它，它活到自身过期（ADR-0003 的取舍）。
func (s *Sessions) Revoke(ctx context.Context, uid uint) error {
	return s.tokens.DeleteByUserID(ctx, uid)
}

// sha256Hex 计算字符串的 SHA-256 十六进制摘要
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// newTokenRaw 生成一枚 refresh token 原文（32 字节随机 → 64 位 hex）。
//
// 签发与轮换共用它：两处的强度要求是同一件事，分开写会变成两份各自漂移的随机源。
func newTokenRaw() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
