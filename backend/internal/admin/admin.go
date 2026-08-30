// Package admin 提供后台管理系统（ADR-0009）：独立鉴权 + 薄 handler + 内嵌前端。
//
// 设计要点：
//   - 后台是与公开 API 分离的独立表面：独立鉴权（不复用公开用户 JWT）、独立路由前缀 /admin/*。
//   - handler 只做协议转换，数据访问复用 backend/service / backend/repository 的现有能力；
//     本包自己的只读接口（UserReader / CommentReader）由 repository 满足（consumer-defined，ADR-0007）。
package admin

import (
	"errors"
)

// 管理端领域错误。与公开端 model 的 sentinel 分开放置，避免后台错误混入公开错误集。
var (
	// ErrCredInvalid 管理端账号或密码错误。
	ErrCredInvalid = errors.New("管理端账号或密码错误")
	// ErrTokenInvalid 管理端认证 token 无效或已过期。
	ErrTokenInvalid = errors.New("管理端认证无效或已过期")
)
