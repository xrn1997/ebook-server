package handler

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"ebook-server/middleware"
	"ebook-server/model"
)

func TestLogHandler_GetList_Empty(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	logHandler := app.log
	router.GET("/api/logs", logHandler.GetList)

	req, _ := http.NewRequest("GET", "/api/logs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 0 {
		t.Errorf("Expected 0 items, got %d", len(items))
	}
}

func TestLogHandler_GetList_WithData(t *testing.T) {
	app := newTestApp(t)

	// 直接通过数据库创建日志数据
	db := app.db
	for i := 0; i < 5; i++ {
		db.Create(&model.OperationLog{
			UserID:       1,
			Username:     "testuser",
			Method:       "GET",
			Path:         "/api/test",
			IP:           "127.0.0.1",
			ResponseCode: 200,
		})
	}

	router := setupRouter()
	logHandler := app.log
	router.GET("/api/logs", logHandler.GetList)

	req, _ := http.NewRequest("GET", "/api/logs?page=1&page_size=3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}
	if data["total"].(float64) != 5 {
		t.Errorf("Expected total 5, got %v", data["total"])
	}
}

func TestLogHandler_GetMyLogs_Success(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	logHandler := app.log
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.GET("/api/logs/my", middleware.JWTAuth(), logHandler.GetMyLogs)

	uid, token := registerUser(t, router, "mylog@example.com")

	// 直接通过数据库创建该用户的日志
	db := app.db
	for i := 0; i < 3; i++ {
		db.Create(&model.OperationLog{
			UserID:       uid,
			Username:     "myloguser",
			Method:       "GET",
			Path:         "/api/test",
			IP:           "127.0.0.1",
			ResponseCode: 200,
		})
	}
	// 创建其他用户的日志
	db.Create(&model.OperationLog{
		UserID:       uid + 1,
		Username:     "other",
		Method:       "POST",
		Path:         "/api/other",
		IP:           "127.0.0.1",
		ResponseCode: 200,
	})

	req, _ := http.NewRequest("GET", "/api/logs/my", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}
}

func TestLogHandler_GetMyLogs_NoAuth(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	logHandler := app.log
	router.GET("/api/logs/my", middleware.JWTAuth(), logHandler.GetMyLogs)

	req, _ := http.NewRequest("GET", "/api/logs/my", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertErrorCode(t, w.Body.Bytes(), "A0230")
}

func TestLogHandler_GetMyLogs_Pagination(t *testing.T) {
	app := newTestApp(t)

	router := setupRouter()
	authHandler := app.auth
	logHandler := app.log
	router.POST("/api/auth/register", authHandler.Register)
	router.POST("/api/auth/login", authHandler.Login)
	router.GET("/api/logs/my", middleware.JWTAuth(), logHandler.GetMyLogs)

	uid, token := registerUser(t, router, "logpage@example.com")

	db := app.db
	for i := 0; i < 10; i++ {
		db.Create(&model.OperationLog{
			UserID:       uid,
			Username:     "logpage",
			Method:       "GET",
			Path:         "/api/test/" + strconv.Itoa(i),
			IP:           "127.0.0.1",
			ResponseCode: 200,
		})
	}

	req, _ := http.NewRequest("GET", "/api/logs/my?page=1&page_size=5", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	data := decodeData(t, w.Body.Bytes())
	items := data["items"].([]interface{})
	if len(items) != 5 {
		t.Errorf("Expected 5 items, got %d", len(items))
	}
	if data["total"].(float64) != 10 {
		t.Errorf("Expected total 10, got %v", data["total"])
	}
}
