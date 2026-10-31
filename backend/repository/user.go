// Package repository 实现数据访问层，是 service 包 Store 接口的 gorm adapter（ADR-0007）。
//
// 每个实例持有自己的 *gorm.DB：连接由 main.go 打开后传入（这里没有包级 DB 变量），
// 测试传入独立 :memory: 实例（pkg/testdb）——repository 自身不读取任何全局变量。
// 请求作用域方法第一个参数是 context.Context，并在 GORM 上使用 WithContext（ADR-0022）。
package repository

import (
	"context"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

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
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// FindByEmail 按邮箱查账号；不存在返回 model.ErrAccountNotFound。
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, translateNotFound(err, model.ErrAccountNotFound)
	}
	return &user, nil
}

// FindByUID 按 UID 查账号；不存在返回 model.ErrAccountNotFound。
func (r *UserRepository) FindByUID(ctx context.Context, uid uint) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).First(&user, uid).Error
	if err != nil {
		return nil, translateNotFound(err, model.ErrAccountNotFound)
	}
	return &user, nil
}

// Update 更新用户
func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

// UpdateProfile 只更新本次提交的展示字段/头像列，不触碰密码/登录计数。
//
// 没提交的列不出现在 UPDATE 里：并发改昵称与头像时，后写的一方不会把前者覆盖回旧值。
func (r *UserRepository) UpdateProfile(ctx context.Context, uid uint, update model.ProfileUpdate) error {
	fields := map[string]any{}
	if update.Username != nil {
		fields["username"] = *update.Username
	}
	if update.Nickname != nil {
		fields["nickname"] = *update.Nickname
	}
	if update.Avatar != nil {
		fields["avatar"] = *update.Avatar
	}
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.User{}).Where("uid = ?", uid).Updates(fields).Error
}

// UpdateAvatar 只更新头像列；注销清空头像时使用。
func (r *UserRepository) UpdateAvatar(ctx context.Context, uid uint, avatar string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("uid = ?", uid).Update("avatar", avatar).Error
}

// CountByAvatar 统计头像仍指向该文件的账号数（含当前账号）。
//
// 比较的是 URL 的文件名而不是整串 URL：头像 URL 的主机取自上传时的请求 Host
// （同一文件可能以 localhost 或内网 IP 两种写法入库），按整串比较会漏掉换主机写法
// 的引用，进而误删别人的文件。文件名比较还必须忽略大小写：Windows/macOS 的文件
// 系统不区分大小写，`A.png` 与 `a.png` 是同一个文件；按字面比较会让攻击者用大写
// 变体绕过引用计数，再触发删除（上传生成的名字全是小写）。反过来，文件名相同的
// 两个不同文件只会让计数偏大、文件多留一会儿——这比误删安全。
//
// 计数**包含当前账号自己**：调用方在 UpdateProfile 之后才调用本方法，此时当前行
// 已是最新 URL。若新旧 URL 只是 host 不同、文件仍是同一个，这一票就会把文件留下；
// 排除自己则会把当前用户正在使用的头像删掉。ClearAvatar 先清空列再调用，自己自然不计。
//
// 头像更新是低频操作，这里读全部非空头像再在 Go 里比较文件名；用户量增长后可改为
// 维护文件名索引，但不要退回整串 URL 的等值比较——那正是漏掉引用、误删文件的来源。
func (r *UserRepository) CountByAvatar(ctx context.Context, avatar string) (int64, error) {
	name := avatarFileName(avatar)
	if name == "" {
		return 0, nil
	}

	var avatars []string
	if err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("avatar <> ''").
		Pluck("avatar", &avatars).Error; err != nil {
		return 0, err
	}

	var count int64
	for _, ref := range avatars {
		if strings.EqualFold(avatarFileName(ref), name) {
			count++
		}
	}
	return count, nil
}

// avatarFileName 取头像 URL 路径的最后一段；无法解析或没有文件名时返回空串。
//
// url.Parse 会解码 %2E 这类转义，因此同一文件的不同转义写法会归一到同一个名字。
func avatarFileName(avatarURL string) string {
	u, err := url.Parse(avatarURL)
	if err != nil {
		return ""
	}
	name := path.Base(u.Path)
	// Windows 的文件名归一化会忽略尾随的 "." 与空格：`x.png.` 打开的就是 x.png。
	// 删除入口（upload.resolveAvatarName）因此拒绝这类名字；引用计数要反过来，
	// 把它们算到真实文件头上——少算一次就可能在别人仍引用时删文件，多算的代价
	// 只是文件多留一会儿。
	name = strings.TrimRight(name, ". ")
	if name == "" || name == "/" {
		return ""
	}
	return name
}

// UpdatePasswordHash 只更新密码列，不触碰资料/登录计数。
func (r *UserRepository) UpdatePasswordHash(ctx context.Context, uid uint, hash string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("uid = ?", uid).Update("password", hash).Error
}

// RecordFailedLogin 一次失败记账：递增计数，并在**同一条 UPDATE** 里按阈值上锁、
// 上锁即清零（锁定期从下一次失败重新计），返回数据库给出的「本行现在是否被锁」。
//
// 判定不靠「先递增、再读回、由 Go 比大小」：读回来的数可能是并发另一个请求的递增，
// 也可能已经被一次并发成功登录清零——那样该锁的没锁、不该锁的提前锁。
// UPDATE 的右值全部取自同一行的旧值，所以 CASE 里的 `login_attempts + 1` 与写入的
// 新计数必然同源；随后那次回读只是转述行的状态，不再参与决策。
func (r *UserRepository) RecordFailedLogin(ctx context.Context, uid uint, maxAttempts int, until time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&model.User{}).Where("uid = ?", uid).
		UpdateColumns(map[string]any{
			// 两个 CASE 的右值都取自旧行，所以「是否达到阈值」在两条赋值之间是同一个判断：
			// 达到就清零并上锁（锁定期从下一次失败重新计），没达到就只累加。
			"login_attempts": gorm.Expr("CASE WHEN login_attempts + 1 >= ? THEN 0 ELSE login_attempts + 1 END", maxAttempts),
			"locked_until": gorm.Expr(
				"CASE WHEN login_attempts + 1 >= ? THEN ? ELSE locked_until END",
				maxAttempts, until,
			),
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		// 记在不存在的账号上不能静默回「没锁」：那等于给暴力破解免掉锁定
		return false, model.ErrAccountNotFound
	}

	var row struct {
		LockedUntil *time.Time
	}
	if err := r.db.WithContext(ctx).Model(&model.User{}).Select("locked_until").
		Where("uid = ?", uid).Scan(&row).Error; err != nil {
		return false, err
	}
	return row.LockedUntil != nil && row.LockedUntil.After(time.Now()), nil
}

// ClearLoginFailures 登录成功后清零失败计数与锁定。
func (r *UserRepository) ClearLoginFailures(ctx context.Context, uid uint) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("uid = ?", uid).Updates(map[string]any{
		"login_attempts": 0,
		"locked_until":   nil,
	}).Error
}

// ExistsByEmail 检查邮箱是否存在
func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}

// Count 统计账号总数（管理后台用）。
func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Count(&count).Error
	return count, err
}

// Search 按关键字分页搜账号（后台「用户搜索」），也是全量列表的唯一入口。
//
// 模糊匹配 email / username / nickname（LIKE 通配符已转义，输入 % 不会退化成全表）；
// keyword 为纯数字时额外按 uid 精确命中——后台最常见的动作就是拿 UID 找人。
// 空 keyword 即不参与过滤（全量），因此不单列一个只差 WHERE 的 FindAll。
// 返回列表与总数；`model.User.Password` 等字段带 `json:"-"`，序列化时不会出网。
func (r *UserRepository) Search(ctx context.Context, keyword string, page, pageSize int) ([]model.User, int64, error) {
	var users []model.User

	query := r.db.WithContext(ctx).Model(&model.User{})
	if keyword != "" {
		pattern := likePattern(keyword)
		cond := r.db.WithContext(ctx).Where("email LIKE ? ESCAPE '\\' OR username LIKE ? ESCAPE '\\' OR nickname LIKE ? ESCAPE '\\'",
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
