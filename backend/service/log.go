package service

import (
	"ebook-server/model"
)

// LogService 操作日志业务服务。
type LogService struct {
	logs LogStore
}

// NewLogService 创建日志服务实例。
func NewLogService(logs LogStore) *LogService {
	return &LogService{
		logs: logs,
	}
}

// Create 创建操作日志
func (s *LogService) Create(log *model.OperationLog) error {
	return s.logs.Create(log)
}

// GetAll 获取所有日志（分页）
func (s *LogService) GetAll(page, pageSize int) (*model.LogListResponse, error) {
	page, pageSize = normalizePage(page, pageSize)

	logs, total, err := s.logs.FindAll(page, pageSize)
	if err != nil {
		return nil, err
	}

	return &model.LogListResponse{
		Items:    logs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetByUserID 根据用户 ID 获取日志
func (s *LogService) GetByUserID(userID uint, page, pageSize int) (*model.LogListResponse, error) {
	page, pageSize = normalizePage(page, pageSize)

	logs, total, err := s.logs.FindByUserID(userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	return &model.LogListResponse{
		Items:    logs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
