package repository

import (
	"context"
	"time"

	"ebook-server/model"

	"gorm.io/gorm"
)

// RefreshTokenRepository 刷新令牌数据访问。满足 service 会话模块（Sessions）的
// refreshTokenStore 能力（轮换语义见 ADR-0003）。
type RefreshTokenRepository struct {
	db *gorm.DB
}

// NewRefreshTokenRepository 创建刷新令牌仓库实例。
func NewRefreshTokenRepository(db *gorm.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

// Create 保存刷新令牌
func (r *RefreshTokenRepository) Create(ctx context.Context, token *model.RefreshToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// FindByHash 按哈希查未过期的刷新令牌；查不到返回 model.ErrLoginExpired。
//
// 查询本身带 `expires_at > now`，所以「没这条记录」与「记录已过期」在数据层是同一个
// 结果，对调用方也都是「请重新登录」——哨兵因此直接选 ErrLoginExpired 而不是某个
// 「记录不存在」：把两者分开的信息在这里没有，硬造一个只会让上层再判一次。
func (r *RefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*model.RefreshToken, error) {
	var token model.RefreshToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND expires_at > ?", hash, time.Now()).
		First(&token).Error
	if err != nil {
		return nil, translateNotFound(err, model.ErrLoginExpired)
	}
	return &token, nil
}

// Rotate 就地轮换一行令牌：只有当前仍是 oldHash 且**尚未过期**时才写新值，返回是否真的改到了行。
//
// 一条 UPDATE 同时承担三件事，这是刻意的，拆成「删旧行 + 插新行」两件就会露出窗口：
//   - 单次消费：两个并发请求拿同一枚令牌查到同一行，先写的一方把哈希换掉，另一方
//     的 CAS 打不中（0 行）必须被拒——否则一枚被盗令牌可以无限次并行兑换。
//   - 撤销优先：Sessions.Revoke 按 user_id 删行，落在本条 UPDATE 之前就是 0 行，
//     轮换失败关闭；换成 INSERT 的话，撤销之后的新行会躲过撤销活满整个 TTL。
//   - 过期优先：`FindByHash` 的 `expires_at > now` 与这条 UPDATE 之间隔着持有者状态
//     检查与随机数生成，令牌可能正好在窗口里到期。不带这个条件的话，一枚刚好到期的
//     令牌会被换成新的 30 天有效期——判过期只能用读到的那一瞬间，不能只靠读。
func (r *RefreshTokenRepository) Rotate(ctx context.Context, id uint, oldHash, newHash string, expiresAt time.Time) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&model.RefreshToken{}).
		Where("id = ? AND token_hash = ? AND expires_at > ?", id, oldHash, time.Now()).
		Updates(map[string]any{"token_hash": newHash, "expires_at": expiresAt})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// DeleteByUserID 删除用户的所有刷新令牌（登出/改密/注销，经 Sessions.Revoke 调用）
func (r *RefreshTokenRepository) DeleteByUserID(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.RefreshToken{}).Error
}

// DeleteExpired 清理过期令牌。
//
// 当前无生产调用方，因此不进 service 的 refreshTokenStore 接口：`FindByHash` 只认
// `expires_at > now` 的行，过期记录对行为完全惰性，回收的只是磁盘（口径见
// docs/development.md「当前已知问题」）。定时清理任务真落地时再扩接口，别提前留一份
// 没人调的对称 API。
func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Where("expires_at <= ?", time.Now()).
		Delete(&model.RefreshToken{}).Error
}
