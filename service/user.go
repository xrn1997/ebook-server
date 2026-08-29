package service

import (
	"ebook-server/model"
	"ebook-server/repository"
)

type UserService struct {
	userRepo *repository.UserRepository
}

func NewUserService() *UserService {
	return &UserService{
		userRepo: repository.NewUserRepository(),
	}
}

// GetByID 根据 ID 获取用户
func (s *UserService) GetByID(id uint) (*model.User, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		if repository.IsRecordNotFound(err) {
			return nil, model.ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

// Update 更新用户信息
func (s *UserService) Update(id uint, req *model.UpdateUserRequest) (*model.User, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		if repository.IsRecordNotFound(err) {
			return nil, model.ErrUserNotFound
		}
		return nil, err
	}

	// 更新字段
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	return user, nil
}
