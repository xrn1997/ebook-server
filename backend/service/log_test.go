package service

import (
	"ebook-server/model"
	"testing"
)

func TestLogService_Create(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	logService := testLogs
	log := &model.OperationLog{
		UserID:       1,
		Username:     "testuser",
		Method:       "GET",
		Path:         "/api/test",
		IP:           "127.0.0.1",
		ResponseCode: 200,
	}

	if err := logService.Create(log); err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	if log.ID == 0 {
		t.Error("Expected non-zero ID after creation")
	}
}

func TestLogService_GetAll_Empty(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	logService := testLogs
	result, err := logService.GetAll(1, 10)
	if err != nil {
		t.Fatalf("Failed to get logs: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("Expected 0 items, got %d", len(result.Items))
	}
	if result.Total != 0 {
		t.Errorf("Expected total 0, got %d", result.Total)
	}
}

func TestLogService_GetAll_WithData(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	logService := testLogs
	for i := 0; i < 15; i++ {
		logService.Create(&model.OperationLog{
			UserID:       1,
			Username:     "testuser",
			Method:       "GET",
			Path:         "/api/test",
			IP:           "127.0.0.1",
			ResponseCode: 200,
		})
	}

	result, err := logService.GetAll(1, 10)
	if err != nil {
		t.Fatalf("Failed to get logs: %v", err)
	}
	if result.Total != 15 {
		t.Errorf("Expected total 15, got %d", result.Total)
	}
	if len(result.Items) != 10 {
		t.Errorf("Expected 10 items, got %d", len(result.Items))
	}
	if result.Page != 1 {
		t.Errorf("Expected page 1, got %d", result.Page)
	}
	if result.PageSize != 10 {
		t.Errorf("Expected pageSize 10, got %d", result.PageSize)
	}

	// 第二页
	result2, err := logService.GetAll(2, 10)
	if err != nil {
		t.Fatalf("Failed to get logs: %v", err)
	}
	if len(result2.Items) != 5 {
		t.Errorf("Expected 5 items on page 2, got %d", len(result2.Items))
	}
}

func TestLogService_GetAll_PaginationBoundaries(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	logService := testLogs
	logService.Create(&model.OperationLog{
		UserID: 1, Username: "u", Method: "GET", Path: "/a", IP: "127.0.0.1", ResponseCode: 200,
	})

	// page=0 应修正为 1
	result, _ := logService.GetAll(0, 10)
	if result.Page != 1 {
		t.Errorf("Expected Page 1 for page=0, got %d", result.Page)
	}

	// pageSize=0 应修正为 10
	result, _ = logService.GetAll(1, 0)
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=0, got %d", result.PageSize)
	}

	// pageSize=101 应修正为 10
	result, _ = logService.GetAll(1, 101)
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=101, got %d", result.PageSize)
	}
}

func TestLogService_GetByUserID(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	logService := testLogs
	for i := 0; i < 8; i++ {
		logService.Create(&model.OperationLog{
			UserID: 1, Username: "user1", Method: "GET", Path: "/a", IP: "127.0.0.1", ResponseCode: 200,
		})
	}
	for i := 0; i < 3; i++ {
		logService.Create(&model.OperationLog{
			UserID: 2, Username: "user2", Method: "POST", Path: "/b", IP: "127.0.0.1", ResponseCode: 200,
		})
	}

	result, err := logService.GetByUserID(1, 1, 10)
	if err != nil {
		t.Fatalf("Failed to get logs: %v", err)
	}
	if result.Total != 8 {
		t.Errorf("Expected total 8, got %d", result.Total)
	}

	result2, _ := logService.GetByUserID(2, 1, 10)
	if result2.Total != 3 {
		t.Errorf("Expected total 3, got %d", result2.Total)
	}

	result3, _ := logService.GetByUserID(999, 1, 10)
	if result3.Total != 0 {
		t.Errorf("Expected total 0, got %d", result3.Total)
	}
}

func TestLogService_GetByUserID_PaginationBoundaries(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	logService := testLogs
	logService.Create(&model.OperationLog{
		UserID: 1, Username: "u", Method: "GET", Path: "/a", IP: "127.0.0.1", ResponseCode: 200,
	})

	// page=0 应修正为 1
	result, _ := logService.GetByUserID(1, 0, 10)
	if result.Page != 1 {
		t.Errorf("Expected Page 1 for page=0, got %d", result.Page)
	}

	// pageSize=0 应修正为 10
	result, _ = logService.GetByUserID(1, 1, 0)
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=0, got %d", result.PageSize)
	}

	// pageSize=101 应修正为 10
	result, _ = logService.GetByUserID(1, 1, 101)
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=101, got %d", result.PageSize)
	}
}
