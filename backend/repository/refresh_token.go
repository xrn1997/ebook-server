package repository

import (
	"context"
	"time"

	"ebook-server/model"

	"gorm.io/gorm"
)

// RefreshTokenRepository 刷新令牌数据访问。满足 service.TokenStore 接口。
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

// DeleteByID 删除指定令牌，返回是否真的删掉了一行。
//
// 刷新轮换必须看 RowsAffected：两个并发请求用同一枚 refresh token 查到同一行时，
// 只有先执行到这个 DELETE 的请求能拿到 true；另一个删到 0 行，必须被拒绝而不是
// 继续签发新令牌（否则一枚被窃取的令牌可以无限次并行兑换）。
func (r *RefreshTokenRepository) DeleteByID(ctx context.Context, id uint) (bool, error) {
	result := r.db.WithContext(ctx).Delete(&model.RefreshToken{}, id)
	return result.RowsAffected > 0, result.Error
}

// DeleteByUserID 删除用户的所有刷新令牌（登出/改密后调用）
func (r *RefreshTokenRepository) DeleteByUserID(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.RefreshToken{}).Error
}

// DeleteExpired 清理过期令牌。
//
// 当前无生产调用方（docs/development.md「当前已知问题」：过期记录暂由数据库承接，
// 暂无定时清理任务），因此不进入 service.TokenStore 接口；清理任务落地时再扩接口。
func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).
		Where("expires_at <= ?", time.Now()).
		Delete(&model.RefreshToken{}).Error
}
