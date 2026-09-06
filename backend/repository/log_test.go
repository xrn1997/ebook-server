package repository

import (
	"ebook-server/model"
	"testing"
)

func TestLogRepository_Create(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewLogRepository(testDB)
	log := &model.OperationLog{
		UserID:       1,
		Username:     "testuser",
		Method:       "POST",
		Path:         "/api/test",
		IP:           "127.0.0.1",
		ResponseCode: 200,
	}

	if err := repo.Create(log); err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	if log.ID == 0 {
		t.Error("Expected non-zero ID after creation")
	}
}

func TestLogRepository_FindAll(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewLogRepository(testDB)
	for i := 0; i < 15; i++ {
		repo.Create(&model.OperationLog{
			UserID:       1,
			Username:     "testuser",
			Method:       "GET",
			Path:         "/api/test",
			IP:           "127.0.0.1",
			ResponseCode: 200,
		})
	}

	// 第一页
	logs, total, err := repo.FindAll(1, 10)
	if err != nil {
		t.Fatalf("Failed to find logs: %v", err)
	}
	if total != 15 {
		t.Errorf("Expected total 15, got %d", total)
	}
	if len(logs) != 10 {
		t.Errorf("Expected 10 logs, got %d", len(logs))
	}

	// 第二页
	logs2, _, _ := repo.FindAll(2, 10)
	if len(logs2) != 5 {
		t.Errorf("Expected 5 logs on page 2, got %d", len(logs2))
	}
}

func TestLogRepository_FindByUserID(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewLogRepository(testDB)
	// 用户1的日志
	for i := 0; i < 8; i++ {
		repo.Create(&model.OperationLog{
			UserID:       1,
			Username:     "user1",
			Method:       "GET",
			Path:         "/api/test",
			IP:           "127.0.0.1",
			ResponseCode: 200,
		})
	}
	// 用户2的日志
	for i := 0; i < 3; i++ {
		repo.Create(&model.OperationLog{
			UserID:       2,
			Username:     "user2",
			Method:       "POST",
			Path:         "/api/test",
			IP:           "127.0.0.1",
			ResponseCode: 200,
		})
	}

	// 查询用户1
	logs, total, err := repo.FindByUserID(1, 1, 10)
	if err != nil {
		t.Fatalf("Failed to find logs: %v", err)
	}
	if total != 8 {
		t.Errorf("Expected total 8, got %d", total)
	}
	if len(logs) != 8 {
		t.Errorf("Expected 8 logs, got %d", len(logs))
	}

	// 查询用户2
	logs2, total2, _ := repo.FindByUserID(2, 1, 10)
	if total2 != 3 {
		t.Errorf("Expected total 3, got %d", total2)
	}
	if len(logs2) != 3 {
		t.Errorf("Expected 3 logs, got %d", len(logs2))
	}

	// 查询不存在的用户
	_, total3, _ := repo.FindByUserID(999, 1, 10)
	if total3 != 0 {
		t.Errorf("Expected total 0, got %d", total3)
	}
}

func TestLogRepository_FindAll_Empty(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewLogRepository(testDB)
	logs, total, err := repo.FindAll(1, 10)
	if err != nil {
		t.Fatalf("Failed to find logs: %v", err)
	}
	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
	if len(logs) != 0 {
		t.Errorf("Expected 0 logs, got %d", len(logs))
	}
}

// TestLogRepository_FindBy 后台日志筛选：方法/路径/账号/业务码/只看失败。
func TestLogRepository_FindBy(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "log@example.com", "log", "log")
	repo := NewLogRepository(testDB)
	repo.Create(&model.OperationLog{UserID: uid, Method: "GET", Path: "/api/comments", ErrorCode: "00000"})
	repo.Create(&model.OperationLog{UserID: uid, Method: "POST", Path: "/api/comments", ErrorCode: "A0303"})
	repo.Create(&model.OperationLog{UserID: uid, Method: "GET", Path: "/api/users/me", ErrorCode: "00000"})
	// 非信封流量（404 等）没有业务码，「只看失败」不该把它们捞出来
	repo.Create(&model.OperationLog{Method: "GET", Path: "/missing", ResponseCode: 404})

	cases := []struct {
		name   string
		filter model.LogFilter
		want   int64
	}{
		{"空调=全量", model.LogFilter{}, 4},
		{"按方法", model.LogFilter{Method: "POST"}, 1},
		{"按路径片段", model.LogFilter{Path: "/api/comments"}, 2},
		{"按账号", model.LogFilter{UserID: uid}, 3},
		{"按业务码", model.LogFilter{ErrorCode: "A0303"}, 1},
		{"只看失败", model.LogFilter{OnlyFailed: true}, 1},
		{"组合条件", model.LogFilter{Method: "GET", Path: "/api"}, 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, total, err := repo.FindBy(c.filter, 1, 10)
			if err != nil {
				t.Fatalf("FindBy failed: %v", err)
			}
			if total != c.want {
				t.Errorf("FindBy(%+v) total = %d, want %d", c.filter, total, c.want)
			}
		})
	}
}
