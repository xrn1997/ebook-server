package service

import (
	"context"
	"fmt"
	"time"

	"ebook-server/model"

	"go.uber.org/zap"
)

// AccountService 账号生命周期业务服务：数据导出与账号注销（ADR-0005）。
//
// 数据导出与账号注销共用同一份导出实现——注销的响应就是「先导出、再匿名化」，
// 两者一起变化，因此放在同一个模块；编排发生在模块内部，handler 只做协议转换。
// 全部依赖由构造函数注入（ADR-0007）。
type AccountService struct {
	users    UserStore
	tokens   TokenStore
	comments CommentStore
	codes    *VerificationCodes
	avatars  AvatarProfile
}

// NewAccountService 创建账号生命周期服务实例。
func NewAccountService(users UserStore, tokens TokenStore, comments CommentStore, codes *VerificationCodes, avatars AvatarProfile) *AccountService {
	return &AccountService{
		users:    users,
		tokens:   tokens,
		comments: comments,
		codes:    codes,
		avatars:  avatars,
	}
}

// Export 导出当前用户的自有数据。
//
// 只含用户资料与自己的评论；刻意不含密码哈希与操作日志——审计记录是服务端
// 对请求的观察，不属于用户自有内容。
// ExportUserData 与注销数据副本历史上各有一份实现，此处合并为唯一一份。
func (s *AccountService) Export(ctx context.Context, uid uint) (*model.UserDataExport, error) {
	user, err := s.users.FindByUID(ctx, uid)
	if err != nil {
		return nil, err
	}
	return s.buildUserDataExport(ctx, user)
}

// SendDeletionCode 向当前账号的邮箱发送注销验证码（ADR-0005）。
//
// 调用方已通过 JWT 认证，账号必然存在，故无需像找回密码那样隐藏账号是否存在。
// 限流键按流程隔离，注销发码不占用注册与找回密码的配额。
func (s *AccountService) SendDeletionCode(ctx context.Context, uid uint) error {
	user, err := s.users.FindByUID(ctx, uid)
	if err != nil {
		return err
	}

	return s.codes.Send(ctx, FlowDeletion, user.Email)
}

// Delete 校验注销验证码后匿名化账号，并返回用户数据副本（ADR-0005）。
//
// 返回的数据副本让用户当场取走自己的内容；服务端不保留可识别信息。
// 注销不可逆——匿名化后无法再按邮箱找到该账号，密码也已清空。
//
// 三次写（清头像列、匿名化、删 refresh token）刻意不包事务：中途失败只会留下
// 「头像已清但账号未匿名化」或「账号已匿名化但 token 未删」的状态——前者用户重新
// 登录即可恢复并重试注销；后者只告警不判失败（数据副本是用户唯一的一份，不能
// 因为一次清理写失败而丢掉），而漏网令牌也续不了期（Refresh 拒绝匿名化账号，见
// auth.go 的 Refresh），只会自行过期。这是有意的取舍，见 docs/development.md 的
// 「当前已知问题」。
func (s *AccountService) Delete(ctx context.Context, uid uint, codeVal string) (*model.UserDataExport, error) {
	user, err := s.users.FindByUID(ctx, uid)
	if err != nil {
		return nil, err
	}

	if err := s.codes.Confirm(ctx, FlowDeletion, user.Email, codeVal); err != nil {
		return nil, err
	}

	// 必须先取数据副本再匿名化，否则拿到的就是占位值
	export, err := s.buildUserDataExport(ctx, user)
	if err != nil {
		return nil, err
	}

	// 头像有 DB 列与磁盘文件两个事实：清空列的同时必须尽力删文件，
	// 否则注销后 /uploads/avatar/* 仍会继续公开旧头像（ADR-0011）。
	// 列更新失败则中止注销；文件删除失败只告警（可能是别的账号共享的 URL 或
	// 权限问题），不回滚已清空的列。用户重新登录即可重试，与上面的自愈语义一致。
	if err := s.avatars.ClearAvatar(ctx, uid); err != nil {
		return nil, err
	}

	anonymizeUser(user)
	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}

	// 立即失效全部 refresh token。刻意放在匿名化之后：并发 Refresh 的轮换若插在
	// 前面几步之间完成，这里能把它刚写入的新令牌一并扫掉。仍漏网的（轮换写入落在
	// 本删除之后）也续不了期——Refresh 对匿名化账号一律拒绝（isAnonymized），
	// 漏网令牌最多存活到自身过期，「注销后无限续期的会话」因此不成立。
	//
	// 失败只告警：此刻账号已经匿名化、数据副本也已构建，把它判成整体失败会让用户
	// 拿不到那份唯一的副本，而漏网令牌本来就没有续期路径。
	if err := s.tokens.DeleteByUserID(ctx, uid); err != nil {
		zap.L().Warn("注销后删除 refresh token 失败，令牌最多存活到自身过期",
			zap.Uint("uid", uid), zap.Error(err))
	}

	return export, nil
}

// buildUserDataExport 构建用户自有数据的完整副本
func (s *AccountService) buildUserDataExport(ctx context.Context, user *model.User) (*model.UserDataExport, error) {
	comments, err := s.comments.FindAllByUserID(ctx, user.UID)
	if err != nil {
		return nil, err
	}

	items := make([]model.ExportedComment, 0, len(comments))
	for _, c := range comments {
		items = append(items, model.ExportedComment{
			ID:          c.ID,
			Content:     c.Content,
			CommentKey:  c.CommentKey,
			ChapterURL:  c.ChapterURL,
			ChapterName: c.ChapterName,
			BookName:    c.BookName,
			CreatedAt:   c.CreatedAt,
		})
	}

	return &model.UserDataExport{
		ExportedAt: time.Now(),
		User: model.UserInfo{
			UID:       user.UID,
			Email:     user.Email,
			Username:  user.Username,
			Nickname:  user.Nickname,
			Avatar:    user.Avatar,
			CreatedAt: user.CreatedAt,
		},
		Comments: items,
	}, nil
}

// anonymizeUser 匿名化账号：抹去可识别信息与凭据，但保留行与公开内容（ADR-0005）。
//
// 刻意不设置 DeletedAt——GORM 的 Preload("User") 会自动附加 WHERE deleted_at IS NULL，
// 一旦软删，评论作者就会被过滤掉，"已注销用户"反而显示不出来。
// 安全性由 password 清空与 email 占位化保证：该账号无法登录，也无法按邮箱检索。
//
// email 改写为 deleted_<uid>@invalid 同时释放了唯一索引，使同一邮箱可以重新注册。
// uid 自增唯一，占位值之间永不冲突；.invalid 是 RFC 2606 保留 TLD，不会与真实邮箱撞车。
func anonymizeUser(user *model.User) {
	user.Email = fmt.Sprintf("deleted_%d@invalid", user.UID)
	user.Username = fmt.Sprintf("已注销用户_%d", user.UID)
	user.Nickname = user.Username
	user.Password = ""
	user.Avatar = ""
	user.LoginAttempts = 0
	user.LockedUntil = nil
}

// isAnonymized 判断账号是否已被匿名化注销（唯一写入方是 anonymizeUser）。
//
// 匿名化清空密码哈希，正常账号必然持有 bcrypt 哈希，空哈希即「已注销」。
// Refresh 续期前必须问一次：注销与并发刷新的竞态可能让轮换写入的令牌漏过
// DeleteByUserID，若它还能继续轮换，就会在账号注销后留下无限续期的会话。
func isAnonymized(user *model.User) bool {
	return user.Password == ""
}
