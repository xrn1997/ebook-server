// Package admin 提供后台管理系统（ADR-0009）：独立鉴权 + 薄 handler + 内嵌前端。
//
// 设计要点：
//   - 后台是与公开 API 分离的独立表面：独立鉴权（不复用公开用户 JWT）、独立路由前缀 /admin/*。
//   - handler 只做协议转换，数据访问复用 backend/service / backend/repository 的现有能力；
//     本包自己的窄接口（UserStore / CommentModule / LogStore）由 repository 或 service 满足
//     （consumer-defined，ADR-0007）。
package admin

import (
	"errors"

	"ebook-server/pkg/errcode"
)

// adminAuthFailureCode 管理端认证失败的统一业务码。
//
// 后台表面用 A0403（后台前端据此重新登录），公开表面用 A0230；两边的差异只由各自
// 的中间件声明，不再把 A0403 散落在 handler 与 auth 的每个失败分支里。
const adminAuthFailureCode = errcode.Forbidden

// 管理端领域错误。与公开端 model 的 sentinel 分开放置，避免后台错误混入公开错误集。
var (
	// ErrCredInvalid 管理端账号或密码错误。
	ErrCredInvalid = errors.New("管理端账号或密码错误")
	// ErrTokenInvalid 管理端认证 token 无效或已过期。
	ErrTokenInvalid = errors.New("管理端认证无效或已过期")
)
