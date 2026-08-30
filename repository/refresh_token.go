package repository

import (
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
func (r *RefreshTokenRepository) Create(token *model.RefreshToken) error {
	return r.db.Create(token).Error
}

// FindByHash 根据哈希查找未过期令牌
func (r *RefreshTokenRepository) FindByHash(hash string) (*model.RefreshToken, error) {
	var token model.RefreshToken
	err := r.db.
		Where("token_hash = ? AND expires_at > ?", hash, time.Now()).
		First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// DeleteByID 删除指定令牌
func (r *RefreshTokenRepository) DeleteByID(id uint) error {
	return r.db.Delete(&model.RefreshToken{}, id).Error
}

// DeleteByUserID 删除用户的所有刷新令牌（登出/改密后调用）
func (r *RefreshTokenRepository) DeleteByUserID(userID uint) error {
	return r.db.
		Where("user_id = ?", userID).
		Delete(&model.RefreshToken{}).Error
}

// DeleteExpired 清理过期令牌。
//
// 当前无生产调用方（AGENTS.md 已知问题：过期记录由数据库接管，暂无定时清理任务），
// 因此不进入 service.TokenStore 接口；定时清理任务落地时再扩接口。
func (r *RefreshTokenRepository) DeleteExpired() error {
	return r.db.
		Where("expires_at <= ?", time.Now()).
		Delete(&model.RefreshToken{}).Error
}
