package service

import (
	"fmt"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/code"
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
	codes    *code.Store             // 校验验证码（注销）
	sender   *VerificationCodeSender // 下发验证码（ADR-0008）
}

// NewAccountService 创建账号生命周期服务实例。
func NewAccountService(users UserStore, tokens TokenStore, comments CommentStore, codes *code.Store, sender *VerificationCodeSender) *AccountService {
	return &AccountService{
		users:    users,
		tokens:   tokens,
		comments: comments,
		codes:    codes,
		sender:   sender,
	}
}

// Export 导出当前用户的自有数据。
//
// 只含用户资料与自己的评论；刻意不含密码哈希与操作日志——
// 操作日志的 RequestBody 可能记录明文密码，见 AGENTS.md 已知问题。
// ExportUserData 与注销数据副本历史上各有一份实现，此处合并为唯一一份。
func (s *AccountService) Export(uid uint) (*model.UserDataExport, error) {
	user, err := s.users.FindByUID(uid)
	if err != nil {
		if IsRecordNotFound(err) {
			return nil, model.ErrUserNotFound
		}
		return nil, err
	}
	return s.buildUserDataExport(user)
}

// SendDeletionCode 向当前账号的邮箱发送注销验证码（ADR-0005）。
//
// 调用方已通过 JWT 认证，账号必然存在，故无需像找回密码那样隐藏账号是否存在。
// 限流键按流程隔离，注销发码不占用注册与找回密码的配额。
func (s *AccountService) SendDeletionCode(uid uint) error {
	user, err := s.users.FindByUID(uid)
	if err != nil {
		if IsRecordNotFound(err) {
			return model.ErrUserNotFound
		}
		return err
	}

	return s.sender.Send(FlowDeletion, user.Email)
}

// Delete 校验注销验证码后匿名化账号，并返回用户数据副本（ADR-0005）。
//
// 返回的数据副本让用户当场取走自己的内容；服务端不保留可识别信息。
// 注销不可逆——匿名化后无法再按邮箱找到该账号，密码也已清空。
//
// 两次写（删 refresh token、匿名化）刻意不包事务：中途失败留下的
// 「token 已删但账号未匿名化」状态可由用户重新登录并重试自愈，
// 详见 AGENTS.md 已知问题。
func (s *AccountService) Delete(uid uint, codeVal string) (*model.UserDataExport, error) {
	user, err := s.users.FindByUID(uid)
	if err != nil {
		if IsRecordNotFound(err) {
			return nil, model.ErrUserNotFound
		}
		return nil, err
	}

	switch s.codes.Verify(FlowDeletion.prefix()+user.Email, codeVal) {
	case code.ResultOK:
		// 校验通过
	case code.ResultTooManyAttempts:
		return nil, model.ErrAttemptTooMany
	default:
		return nil, model.ErrCodeInvalid
	}

	// 必须先取数据副本再匿名化，否则拿到的就是占位值
	export, err := s.buildUserDataExport(user)
	if err != nil {
		return nil, err
	}

	// 立即失效全部 refresh token
	if err := s.tokens.DeleteByUserID(uid); err != nil {
		return nil, err
	}

	anonymizeUser(user)
	if err := s.users.Update(user); err != nil {
		return nil, err
	}

	return export, nil
}

// buildUserDataExport 构建用户自有数据的完整副本
func (s *AccountService) buildUserDataExport(user *model.User) (*model.UserDataExport, error) {
	comments, err := s.comments.FindAllByUserID(user.UID)
	if err != nil {
		return nil, err
	}

	items := make([]model.ExportedComment, 0, len(comments))
	for _, c := range comments {
		items = append(items, model.ExportedComment{
			ID:        c.ID,
			Content:   c.Content,
			CreatedAt: c.CreatedAt,
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
