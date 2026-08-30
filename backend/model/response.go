package model

// Response 统一响应信封
//
// Code 为五位字符串业务码（"00000" 表示成功），HTTP 状态码恒为 200，
// 业务层的成功与失败都通过该信封传达，避免移动端对非 2xx 响应直接抛异常导致业务文案丢失。
type Response struct {
	Code  string      `json:"code"`  // 五位业务码，"00000" 表示成功
	Error string      `json:"error"` // 业务错误文案（成功时为空串）
	Data  interface{} `json:"data,omitempty"`
}
