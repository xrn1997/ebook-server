package repository

import (
	"ebook-server/model"

	"gorm.io/gorm"
)

// LogRepository 操作日志数据访问。满足 service.LogStore 接口。
type LogRepository struct {
	db *gorm.DB
}

// NewLogRepository 创建日志仓库实例。
func NewLogRepository(db *gorm.DB) *LogRepository {
	return &LogRepository{db: db}
}

// Create 创建操作日志
func (r *LogRepository) Create(log *model.OperationLog) error {
	return r.db.Create(log).Error
}

// FindAll 查找所有日志（分页）
func (r *LogRepository) FindAll(page, pageSize int) ([]model.OperationLog, int64, error) {
	var logs []model.OperationLog
	var total int64

	query := r.db.Model(&model.OperationLog{})

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// FindByUserID 根据用户 ID 查找日志
func (r *LogRepository) FindByUserID(userID uint, page, pageSize int) ([]model.OperationLog, int64, error) {
	var logs []model.OperationLog
	var total int64

	query := r.db.Model(&model.OperationLog{}).Where("user_id = ?", userID)

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
