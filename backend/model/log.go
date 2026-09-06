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

// LogFilter 操作日志筛选条件（后台「日志筛选」）。
//
// 零值字段一律不参与过滤，因此空调 = 全量列表。用结构而非四个平铺参数：这些条件
// 总是一起动，以后加维度（时间段、状态码区间）不该改一遍所有调用方的签名。
type LogFilter struct {
	Method     string // HTTP 方法精确匹配，如 GET / POST
	Path       string // 路径片段模糊匹配
	UserID     uint   // 按账号过滤，0 = 不限
	ErrorCode  string // 信封业务码精确匹配，如 A0303
	OnlyFailed bool   // 只看业务失败的请求
}
