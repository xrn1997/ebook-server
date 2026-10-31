package model

import "time"

// OperationLog 操作日志实体（GORM 模型）。
//
// 记录每次 API 请求的元信息，用于审计与排障。
// RequestBody 刻意不填（登录等请求体含明文密码）；错误时 ErrorCode/ErrorMessage
// 记录统一信封里的业务码与文案（HTTP 恒为 200，单看状态码无法区分成败）。
//
// 事实字段嵌入 OperationRecord：两者描述同一次操作，字段清单只此一份，
// 加维度时实体与审计输入不会各漂各的。
type OperationLog struct {
	ID uint `json:"id" gorm:"primaryKey"`
	OperationRecord
	RequestBody string    `json:"request_body" gorm:"type:text"`
	// CreatedAt 挂索引：后台日志列表按 created_at DESC 分页（repository/log.go），
	// operation_logs 是最大的表，没有索引时翻页退化为全表扫描 + 排序。
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

// TableName 指定表名
func (OperationLog) TableName() string {
	return "operation_logs"
}

// OperationRecord 一次客户端操作的事实（审计模块的输入）。
//
// 写入方是 HTTP 中间件，但「哪些请求算一次操作」是操作日志模块的域规则：
// 中间件只交出协议层能拿到的事实，不判断该不该记。这样规则不随接入点变化
// 而复制——路由表调整、多表面挂载都不影响判断。
//
// 字段上挂着 json/gorm 标注：本结构经嵌入成为 OperationLog 的表列与响应字段，
// 标注只此一份（字段名与列名、JSON 键逐一同名，嵌入后语义不变）。
type OperationRecord struct {
	UserID       uint   `json:"user_id" gorm:"index"`
	Username     string `json:"username" gorm:"size:50"`
	Method       string `json:"method" gorm:"size:10"`
	Path         string `json:"path" gorm:"size:255"`
	IP           string `json:"ip" gorm:"size:50"`
	UserAgent    string `json:"user_agent" gorm:"type:text"`
	ResponseCode int    `json:"response_code"`
	// ErrorCode 统一信封业务码，"00000" 表示成功
	ErrorCode    string `json:"error_code" gorm:"size:10"`
	ErrorMessage string `json:"error_message" gorm:"type:text"`
}

// 审计字段的落库上限（字节）。HTTP 头与请求行都能到 MB 级，而公开的
// GET /api/comments 也经过审计：不设上限时一个匿名请求就能往 SQLite 写一条
// 接近 1 MB 的行，形成廉价的磁盘填充与写锁放大。写入方（LogService.Record）
// 统一截断，中间件只负责交事实。
const (
	MaxOperationLogPathLength         = 255
	MaxOperationLogUserAgentLength    = 512
	MaxOperationLogErrorMessageLength = 1024
)

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
	Method string // HTTP 方法精确匹配，如 GET / POST
	Path   string // 路径片段模糊匹配
	// UserID 按账号过滤，0 = 不限。
	//
	// 这里拿 0 当哨兵是安全的，但别照着它抄到别的表面：本结构只服务后台的
	// /admin/api/logs（ADR-0010，只有本机可达），而后台看的人本来就能看全量日志，
	// 「0 被误当不限」不会把任何新数据交出去。评论那条读路径在公开表面上，所以
	// model.CommentFilter 刻意用 ByUserID 显式表达同一件事。
	UserID uint
	// ErrorCode 信封业务码精确匹配，如 A0303
	ErrorCode string
	// OnlyFailed 只看业务失败的请求
	OnlyFailed bool
}
