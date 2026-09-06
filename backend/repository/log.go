package repository

import (
	"ebook-server/model"
	"ebook-server/pkg/errcode"

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
	return r.FindBy(model.LogFilter{}, page, pageSize)
}

// FindBy 按筛选条件分页查操作日志（后台「日志筛选」）。
//
// 零值字段不参与过滤，因此空调 = 全量列表。路径用模糊匹配（审计时通常只记得片段），
// 方法/账号用精确匹配；OnlyFailed 筛业务码非 00000 的请求——HTTP 恒为 200，
// 成败只能看信封里的业务码（见 ADR-0001）。
func (r *LogRepository) FindBy(filter model.LogFilter, page, pageSize int) ([]model.OperationLog, int64, error) {
	var logs []model.OperationLog

	query := r.db.Model(&model.OperationLog{})
	if filter.Method != "" {
		query = query.Where("method = ?", filter.Method)
	}
	if filter.Path != "" {
		query = query.Where("path LIKE ? ESCAPE '\\'", likePattern(filter.Path))
	}
	if filter.UserID != 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.ErrorCode != "" {
		query = query.Where("error_code = ?", filter.ErrorCode)
	}
	if filter.OnlyFailed {
		// 业务码非空且非 00000 才算业务失败：404、静态资源等非信封响应没有业务码，
		// 若只写 <> '00000' 会把它们一并捞出来。
		query = query.Where("error_code <> '' AND error_code <> ?", errcode.SuccessCode)
	}

	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &logs)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// FindByUserID 根据用户 ID 查找日志
func (r *LogRepository) FindByUserID(userID uint, page, pageSize int) ([]model.OperationLog, int64, error) {
	var logs []model.OperationLog

	query := r.db.Model(&model.OperationLog{}).Where("user_id = ?", userID)
	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &logs)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
