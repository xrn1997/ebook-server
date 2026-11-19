// Package admin 提供后台管理系统（ADR-0009）：与公开 API 分离的独立表面——
// 独立鉴权（不复用公开用户 JWT）、独立路由前缀 /admin/*；handler 只做协议转换，
// 数据访问经本包的窄接口（UserStore / CommentModule / LogStore）复用 backend/service
// 与 backend/repository 的现有能力（consumer-defined，ADR-0007）。
package admin

import (
	"errors"

	"ebook-server/pkg/errcode"
)

// adminAuthFailureCode 管理端认证失败的统一业务码：后台表面用 A0403（后台前端据此
// 重新登录），公开表面用 A0230，两边的差异只由各自的中间件声明。
//
// 裁决：不并入 errcode 的 errorCodes 表（那张表只登记 model 哨兵到业务码的映射）——
// 塞进来需要先造一枚只服务后台的 model 哨兵，等于把后台错误混进公开错误集。
const adminAuthFailureCode = errcode.Forbidden

// 管理端领域错误：与公开端 model 的 sentinel 分开放置，避免后台错误混入公开错误集，
// 也因此不进 errcode 的 errorCodes 表（表的条目都是 model 哨兵）。后台响应的码与
// 文案由调用点显式给出（adminAuthFailureCode + sentinel 的 Error()）。
var (
	// ErrCredInvalid 管理端账号或密码错误。
	ErrCredInvalid = errors.New("管理端账号或密码错误")
	// ErrTokenInvalid 管理端认证 token 无效或已过期。
	ErrTokenInvalid = errors.New("管理端认证无效或已过期")
)
