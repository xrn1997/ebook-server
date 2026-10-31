package model

// SuccessCode 统一信封的业务成功码。
//
// 方案上的成功判定（例如操作日志的「只看失败」筛选）在数据层也要用，而数据层
// 不该 import HTTP 响应包；成功码因此住在最底层的 model，pkg/errcode 以别名引用。
const SuccessCode = "00000"

// Response 统一响应信封
//
// Code 为五位字符串业务码（"00000" 表示成功），HTTP 状态码恒为 200，
// 业务层的成功与失败都通过该信封传达，避免移动端对非 2xx 响应直接抛异常导致业务文案丢失。
type Response struct {
	Code  string      `json:"code"`  // 五位业务码，"00000" 表示成功
	Error string      `json:"error"` // 业务错误文案（成功时为空串）
	Data  interface{} `json:"data,omitempty"`
}
