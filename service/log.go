package service

import (
	"ebook-server/model"
	"ebook-server/repository"
)

type LogService struct {
	logRepo *repository.LogRepository
}

func NewLogService() *LogService {
	return &LogService{
		logRepo: repository.NewLogRepository(),
	}
}

// Create 创建操作日志
func (s *LogService) Create(log *model.OperationLog) error {
	return s.logRepo.Create(log)
}

// GetAll 获取所有日志（分页）
func (s *LogService) GetAll(page, pageSize int) (*model.LogListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	logs, total, err := s.logRepo.FindAll(page, pageSize)
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
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	logs, total, err := s.logRepo.FindByUserID(userID, page, pageSize)
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
