package handler_test

import (
	"net/http"
	"strconv"
	"testing"

	"ebook-server/model"
)

func TestLogHandler_GetMyLogs_Success(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	uid, token := registerUser(t, router, "mylog@example.com")

	// 直接通过数据库创建该用户的日志（审计写入无公开入口，造数只能直连库）
	db := app.db
	for i := 0; i < 3; i++ {
		db.Create(&model.OperationLog{
			OperationRecord: model.OperationRecord{
				UserID:       uid,
				Username:     "myloguser",
				Method:       "GET",
				Path:         "/api/test",
				IP:           "127.0.0.1",
				ResponseCode: 200,
			},
		})
	}
	// 创建其他用户的日志
	db.Create(&model.OperationLog{
		OperationRecord: model.OperationRecord{
			UserID:       uid + 1,
			Username:     "other",
			Method:       "POST",
			Path:         "/api/other",
			IP:           "127.0.0.1",
			ResponseCode: 200,
		},
	})

	w := doJSON(t, router, http.MethodGet, "/api/logs/my", nil, token)
	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}
}

func TestLogHandler_GetMyLogs_NoAuth(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	w := doJSON(t, router, http.MethodGet, "/api/logs/my", nil, "")
	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestLogHandler_GetMyLogs_Pagination(t *testing.T) {
	app := newTestApp(t)
	router := app.engine

	uid, token := registerUser(t, router, "logpage@example.com")

	db := app.db
	for i := 0; i < 10; i++ {
		db.Create(&model.OperationLog{
			OperationRecord: model.OperationRecord{
				UserID:       uid,
				Username:     "logpage",
				Method:       "GET",
				Path:         "/api/test/" + strconv.Itoa(i),
				IP:           "127.0.0.1",
				ResponseCode: 200,
			},
		})
	}

	w := doJSON(t, router, http.MethodGet, "/api/logs/my?page=1&page_size=5", nil, token)
	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 5 {
		t.Errorf("Expected 5 items, got %d", len(items))
	}
	if data["total"].(float64) != 10 {
		t.Errorf("Expected total 10, got %v", data["total"])
	}
}
