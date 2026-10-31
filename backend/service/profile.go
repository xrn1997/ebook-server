package service

import (
	"context"

	"ebook-server/model"

	"go.uber.org/zap"
)

// ProfileService 账号资料的唯一写入口：展示字段更新与头像文件生命周期。
//
// 头像有两个事实：users.avatar 的 URL 与 uploads/avatar 下的文件。更新资料
// 与注销账号都必须同时处理两者，因此收进同一个模块；调用方只递意图。
type ProfileService struct {
	users   ProfileStore
	avatars AvatarCleaner
}

// NewProfileService 创建账号资料服务实例。
func NewProfileService(users ProfileStore, avatars AvatarCleaner) *ProfileService {
	return &ProfileService{
		users:   users,
		avatars: avatars,
	}
}

// Get 按 UID 取账号；账号不存在时透出 repository 给的 model.ErrAccountNotFound。
func (s *ProfileService) Get(ctx context.Context, uid uint) (*model.User, error) {
	return s.users.FindByUID(ctx, uid)
}

// Update 更新账号资料（部分更新语义：空串表示不修改）。
func (s *ProfileService) Update(ctx context.Context, uid uint, req *model.UpdateUserRequest) (*model.User, error) {
	user, err := s.users.FindByUID(ctx, uid)
	if err != nil {
		return nil, err
	}

	// 登录主标识不可变更（ADR-0004）。
	// 传了不同的值 → 明确拒绝；传了相同的值 → 幂等忽略。
	// 不能静默丢弃：Gin 的 ShouldBindJSON 会忽略未知字段，客户端会误以为修改成功。
	if req.Email != "" && req.Email != user.Email {
		return nil, model.ErrEmailImmutable
	}

	oldAvatar := user.Avatar

	var update model.ProfileUpdate
	if req.Username != "" {
		username := req.Username
		update.Username = &username
	}
	if req.Nickname != "" {
		nickname := req.Nickname
		update.Nickname = &nickname
	}
	avatar := user.Avatar
	if req.Avatar != "" {
		avatar = req.Avatar
		nextAvatar := req.Avatar
		update.Avatar = &nextAvatar
	}

	// 只写本次提交的列：整行 Save 会把并发的登录计数/密码写回旧值，
	// 把三个字段一起写回则会让并发改不同字段的请求互相覆盖。
	if err := s.users.UpdateProfile(ctx, uid, update); err != nil {
		return nil, err
	}

	// 清理旧头像文件（ADR-0011）：仅当本次确实更换了头像时删除旧文件。
	if req.Avatar != "" && oldAvatar != "" && oldAvatar != avatar {
		s.deleteAvatarIfUnreferenced(ctx, oldAvatar)
	}

	// 重新读回而不是拼内存对象：updated_at 由数据库在 UPDATE 时改写，
	// 沿用旧对象会把过期时间原样返回给客户端。
	return s.users.FindByUID(ctx, uid)
}

// ClearAvatar 清空头像列并删除本服务文件；注销流程在匿名化之前调用。
func (s *ProfileService) ClearAvatar(ctx context.Context, uid uint) error {
	user, err := s.users.FindByUID(ctx, uid)
	if err != nil {
		return err
	}
	if err := s.users.UpdateAvatar(ctx, uid, ""); err != nil {
		return err
	}
	s.deleteAvatarIfUnreferenced(ctx, user.Avatar)
	return nil
}

// deleteAvatarIfUnreferenced 删除本服务的旧头像文件，但先确认没有账号仍指向它。
//
// 头像 URL 是公开的（评论列表会带出 user.avatar），而资料更新接受客户端提供的 URL：
// 不查引用就删文件，等于允许任何人把别人的头像填进自己的资料、再换一次头像/
// 注销账号，删掉对方的文件。查询失败时选择不删——宁可留下孤儿文件，也不误删。
//
// 引用计数按文件名比较且包含当前账号（见 ProfileStore.CountByAvatar）：UpdateProfile
// 已经写入新 URL，若新旧 URL 只是 host 不同、文件仍是同一个，这一票会把文件留下。
// DeleteAvatar 只删本服务 uploads 内文件，外部 URL no-op；删除失败只告警。
func (s *ProfileService) deleteAvatarIfUnreferenced(ctx context.Context, avatarURL string) {
	if avatarURL == "" {
		return
	}
	refs, err := s.users.CountByAvatar(ctx, avatarURL)
	if err != nil {
		zap.L().Warn("count avatar references failed; keep file", zap.String("url", avatarURL), zap.Error(err))
		return
	}
	if refs > 0 {
		return
	}
	if err := s.avatars.DeleteAvatar(avatarURL); err != nil {
		zap.L().Warn("delete avatar file failed", zap.String("url", avatarURL), zap.Error(err))
	}
}
