package model

import "errors"

var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUsernameExists     = errors.New("用户名已存在")
	ErrUserNotFound       = errors.New("用户不存在")
	ErrCommentNotFound    = errors.New("评论不存在")
	ErrNoPermission       = errors.New("无权限操作")
	ErrInvalidToken       = errors.New("无效的令牌")
	// ErrCommentNotOwner 删除评论时发现非本人（ADR-0011：仅本人可删，映射 A0303）。
	// 与通用 ErrNoPermission 分离：评论域的「非本人」是专门业务语义，客户端需据此提示。
	ErrCommentNotOwner = errors.New("无权删除该评论")
	// ErrCommentKeySame 迁移评论聚合键时新旧键相同（映射 A0305）。
	ErrCommentKeySame = errors.New("新旧聚合键相同")

	// 认证/账号体系错误（ADR-0001 / ADR-0002）
	ErrAccountNotFound = errors.New("账户不存在")
	ErrAccountLocked   = errors.New("登录失败次数过多，账号已暂时锁定")
	ErrEmailExists     = errors.New("该邮箱已被注册")
	// ErrEmailImmutable 登录主标识不可变更（ADR-0004）。换邮箱需注销后重新注册。
	ErrEmailImmutable = errors.New("登录主标识不可变更，如需更换请注销后重新注册")
	ErrPasswordWrong  = errors.New("密码错误")
	ErrTokenInvalid   = errors.New("token 无效")
	ErrLoginExpired   = errors.New("登录已过期，请重新登录")
	ErrCodeInvalid    = errors.New("验证码错误或已过期")
	ErrAttemptTooMany = errors.New("尝试次数超限")
	ErrMailSendFailed = errors.New("邮件发送失败")
)
