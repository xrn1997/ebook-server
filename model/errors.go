package model

import "errors"

var (
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUsernameExists     = errors.New("用户名已存在")
	ErrUserNotFound       = errors.New("用户不存在")
	ErrCommentNotFound    = errors.New("评论不存在")
	ErrNoPermission       = errors.New("无权限操作")
	ErrInvalidToken       = errors.New("无效的令牌")
)
