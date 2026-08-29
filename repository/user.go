package repository

import (
	"ebook-server/model"
	"ebook-server/pkg/database"

	"gorm.io/gorm"
)

type UserRepository struct{}

func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

// Create 创建用户
func (r *UserRepository) Create(user *model.User) error {
	return database.GetDB().Create(user).Error
}

// FindByUsername 根据用户名查找用户
func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := database.GetDB().Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByID 根据 ID 查找用户
func (r *UserRepository) FindByID(id uint) (*model.User, error) {
	var user model.User
	err := database.GetDB().First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update 更新用户
func (r *UserRepository) Update(user *model.User) error {
	return database.GetDB().Save(user).Error
}

// Delete 软删除用户
func (r *UserRepository) Delete(id uint) error {
	return database.GetDB().Delete(&model.User{}, id).Error
}

// ExistsByUsername 检查用户名是否存在
func (r *UserRepository) ExistsByUsername(username string) (bool, error) {
	var count int64
	err := database.GetDB().Model(&model.User{}).Where("username = ?", username).Count(&count).Error
	return count > 0, err
}

// IsRecordNotFound 检查错误是否为记录未找到
func IsRecordNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}

// HasError 检查是否有错误（非记录未找到）
func HasError(err error) bool {
	return err != nil && err != gorm.ErrRecordNotFound
}
