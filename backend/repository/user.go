// Package repository 实现数据访问层，是 service 包 Store 接口的 gorm adapter（ADR-0007）。
//
// 每个实例持有自己的 *gorm.DB：生产由 main.go 传入全局连接，测试传入独立
// :memory: 实例（pkg/testdb）——repository 自身不读取任何全局变量。
package repository

import (
	"strconv"

	"ebook-server/model"

	"gorm.io/gorm"
)

// UserRepository 账号数据访问。满足 service.UserStore 接口。
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建账号仓库实例。
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create 创建用户
func (r *UserRepository) Create(user *model.User) error {
	return r.db.Create(user).Error
}

// FindByEmail 根据邮箱查找用户
func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var user model.User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByUID 根据 UID 查找用户
func (r *UserRepository) FindByUID(uid uint) (*model.User, error) {
	var user model.User
	err := r.db.First(&user, uid).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update 更新用户
func (r *UserRepository) Update(user *model.User) error {
	return r.db.Save(user).Error
}

// ExistsByEmail 检查邮箱是否存在
func (r *UserRepository) ExistsByEmail(email string) (bool, error) {
	var count int64
	err := r.db.Model(&model.User{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}

// Count 统计账号总数（管理后台用）。
func (r *UserRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&model.User{}).Count(&count).Error
	return count, err
}

// FindAll 分页列出账号（管理后台用）。
//
// 返回列表与总数；`model.User.Password` 等字段带 `json:"-"`，序列化时不会出网。
func (r *UserRepository) FindAll(page, pageSize int) ([]model.User, int64, error) {
	var users []model.User
	total, err := paginateQuery(r.db.Model(&model.User{}), "created_at DESC", page, pageSize, &users)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// Search 按关键字分页搜账号（后台「用户搜索」）。
//
// 模糊匹配 email / username / nickname（LIKE 通配符已转义，输入 % 不会退化成全表）；
// keyword 为纯数字时额外按 uid 精确命中——后台最常见的动作就是拿 UID 找人。
// 空 keyword 与 FindAll 同义（不参与过滤），调用方无需自己分支。
func (r *UserRepository) Search(keyword string, page, pageSize int) ([]model.User, int64, error) {
	var users []model.User

	query := r.db.Model(&model.User{})
	if keyword != "" {
		pattern := likePattern(keyword)
		cond := r.db.Where("email LIKE ? ESCAPE '\\' OR username LIKE ? ESCAPE '\\' OR nickname LIKE ? ESCAPE '\\'",
			pattern, pattern, pattern)
		if uid, err := strconv.ParseUint(keyword, 10, 64); err == nil {
			cond = cond.Or("uid = ?", uid)
		}
		query = query.Where(cond)
	}

	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &users)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// IsRecordNotFound 检查错误是否为记录未找到。
//
// gorm 的哨兵错误经此一个函数暴露给 service 层，是 gorm 类型越过 seam 的唯一窄口；
// 引入错误翻译（ADR-0007 被拒方案）前不要在 service 层直接引用 gorm。
func IsRecordNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}
