package service

import (
	"ebook-server/model"

	"go.uber.org/zap"
)

// UserService 用户信息业务服务：资料查看与更新。
//
// 头像文件清理（ADR-0011）在业务层编排：更换头像成功后删除本服务旧文件，
// 不放在 handler——文件生命周期是业务规则，handler 只做协议转换（AGENTS.md 分层）。
type UserService struct {
	users   UserStore
	avatars AvatarCleaner
}

// NewUserService 创建用户服务实例。
func NewUserService(users UserStore, avatars AvatarCleaner) *UserService {
	return &UserService{
		users:   users,
		avatars: avatars,
	}
}

// GetByUID 根据 UID 获取用户
func (s *UserService) GetByUID(uid uint) (*model.User, error) {
	user, err := s.users.FindByUID(uid)
	if err != nil {
		if IsRecordNotFound(err) {
			return nil, model.ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

// Update 更新用户信息
func (s *UserService) Update(uid uint, req *model.UpdateUserRequest) (*model.User, error) {
	user, err := s.users.FindByUID(uid)
	if err != nil {
		if IsRecordNotFound(err) {
			return nil, model.ErrUserNotFound
		}
		return nil, err
	}

	// 登录主标识不可变更（ADR-0004）。
	// 传了不同的值 → 明确拒绝；传了相同的值 → 幂等忽略。
	// 不能静默丢弃：Gin 的 ShouldBindJSON 会忽略未知字段，客户端会误以为修改成功。
	if req.Email != "" && req.Email != user.Email {
		return nil, model.ErrEmailImmutable
	}

	oldAvatar := user.Avatar

	// 更新字段
	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Nickname != "" {
		user.Nickname = req.Nickname
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}

	if err := s.users.Update(user); err != nil {
		return nil, err
	}

	// 清理旧头像文件（ADR-0011）：仅当本次确实更换了头像（新值非空且与旧值不同）时
	// 删除旧文件；DeleteAvatar 只删本服务 uploads 内文件，外部 URL no-op。
	// 删除失败只告警不阻断——旧文件成孤儿由运维定期清理，不引入事务复杂度。
	if oldAvatar != "" && oldAvatar != user.Avatar {
		if err := s.avatars.DeleteAvatar(oldAvatar); err != nil {
			zap.L().Warn("delete old avatar file failed", zap.String("url", oldAvatar), zap.Error(err))
		}
	}

	return user, nil
}
