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

// FindByEmail 根据邮箱查找用户
func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var user model.User
	err := database.GetDB().Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByUID 根据 UID 查找用户
func (r *UserRepository) FindByUID(uid uint) (*model.User, error) {
	var user model.User
	err := database.GetDB().First(&user, uid).Error
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
func (r *UserRepository) Delete(uid uint) error {
	return database.GetDB().Delete(&model.User{}, uid).Error
}

// ExistsByEmail 检查邮箱是否存在
func (r *UserRepository) ExistsByEmail(email string) (bool, error) {
	var count int64
	err := database.GetDB().Model(&model.User{}).Where("email = ?", email).Count(&count).Error
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
