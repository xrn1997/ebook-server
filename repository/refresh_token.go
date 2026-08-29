package repository

import (
	"ebook-server/model"
	"ebook-server/pkg/database"
	"time"
)

// RefreshTokenRepository 刷新令牌数据访问。
type RefreshTokenRepository struct{}

// NewRefreshTokenRepository 创建刷新令牌仓库实例。
func NewRefreshTokenRepository() *RefreshTokenRepository {
	return &RefreshTokenRepository{}
}

// Create 保存刷新令牌
func (r *RefreshTokenRepository) Create(token *model.RefreshToken) error {
	return database.GetDB().Create(token).Error
}

// FindByHash 根据哈希查找未过期令牌
func (r *RefreshTokenRepository) FindByHash(hash string) (*model.RefreshToken, error) {
	var token model.RefreshToken
	err := database.GetDB().
		Where("token_hash = ? AND expires_at > ?", hash, time.Now()).
		First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// DeleteByID 删除指定令牌
func (r *RefreshTokenRepository) DeleteByID(id uint) error {
	return database.GetDB().Delete(&model.RefreshToken{}, id).Error
}

// DeleteByUserID 删除用户的所有刷新令牌（登出/改密后调用）
func (r *RefreshTokenRepository) DeleteByUserID(userID uint) error {
	return database.GetDB().
		Where("user_id = ?", userID).
		Delete(&model.RefreshToken{}).Error
}

// DeleteExpired 清理过期令牌
func (r *RefreshTokenRepository) DeleteExpired() error {
	return database.GetDB().
		Where("expires_at <= ?", time.Now()).
		Delete(&model.RefreshToken{}).Error
}
