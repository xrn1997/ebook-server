package model

import "time"

// OperationLog 操作日志实体（GORM 模型）。
//
// 记录每次 API 请求的元信息，用于审计与排障。
// RequestBody 刻意不填（登录等请求体含明文密码）；错误时 ErrorCode/ErrorMessage
// 记录统一信封里的业务码与文案（HTTP 恒为 200，单看状态码无法区分成败）。
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
	ErrorCode    string    `json:"error_code" gorm:"size:10"` // 统一信封业务码，"00000" 表示成功
	ErrorMessage string    `json:"error_message" gorm:"type:text"`
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
