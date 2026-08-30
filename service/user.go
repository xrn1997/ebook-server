package service

import (
	"ebook-server/model"
)

// UserService 用户信息业务服务：资料查看与更新。
type UserService struct {
	users UserStore
}

// NewUserService 创建用户服务实例。
func NewUserService(users UserStore) *UserService {
	return &UserService{
		users: users,
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

	return user, nil
}
