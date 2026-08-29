package model

import "time"

type OperationLog struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	UserID       uint      `json:"user_id" gorm:"index"`
	Username     string    `json:"username" gorm:"size:50"`
	Method       string    `json:"method" gorm:"size:10"`
	Path         string    `json:"path" gorm:"size:255"`
	IP           string    `json:"ip" gorm:"size:50"`
	UserAgent    string    `json:"user_agent" gorm:"type:text"`
	RequestBody  string    `json:"request_body" gorm:"type:text"`
	ResponseCode int       `json:"response_code"`
	CreatedAt    time.Time `json:"created_at"`
}

// TableName 指定表名
func (OperationLog) TableName() string {
	return "operation_logs"
}

// LogListResponse 日志列表响应
type LogListResponse struct {
	Items    []OperationLog `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}
