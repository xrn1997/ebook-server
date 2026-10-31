package model

import "errors"

var (
	ErrCommentNotFound = errors.New("评论不存在")
	// ErrCommentNotOwner 删除评论时发现非本人（ADR-0011：仅本人可删，映射 A0303）。
	// 与通用「无权限」分离：评论域的「非本人」是专门业务语义，客户端需据此提示。
	ErrCommentNotOwner = errors.New("无权删除该评论")
	// ErrCommentKeySame 迁移评论聚合键时新旧键相同（映射 A0305）。
	ErrCommentKeySame = errors.New("新旧聚合键相同")
	// ErrCommentKeyInvalid 改键的键本身不合规（目标键为空、全员范围内旧键为空、超长）。
	// 映射 A0400：与 binding 拒绝同一形态，客户端不必区分是谁挡下的。
	// 产生方用 %w 带上具体理由，业务码由 errcode 沿包装链查表（ADR-0001）。
	ErrCommentKeyInvalid = errors.New("聚合键不合规")
	// ErrCommentFilterInvalid 评论列表的过滤条件不合规（键数或单键长度超上限）。
	// 上限存在的理由是「一条语句的绑定变量总数」，属查询语义而非某个端点的细节。
	ErrCommentFilterInvalid = errors.New("评论过滤条件不合规")

	// 认证/账号体系错误（ADR-0001 / ADR-0002）
	// ErrAccountNotFound 账号查不到——按邮箱与按 UID 两条取路共用这一枚，
	// 由 repository 在边界上给出（映射 A0201；未认证端点要改码防枚举，见 ADR-0006）。
	ErrAccountNotFound = errors.New("账户不存在")
	ErrAccountLocked   = errors.New("登录失败次数过多，账号已暂时锁定")
	ErrEmailExists     = errors.New("该邮箱已被注册")
	// ErrEmailImmutable 登录主标识不可变更（ADR-0004）。换邮箱需注销后重新注册。
	ErrEmailImmutable = errors.New("登录主标识不可变更，如需更换请注销后重新注册")
	ErrPasswordWrong  = errors.New("密码错误")
	// ErrPasswordInvalid 密码超出凭据模块的硬约束（bcrypt 只接受至多 72 字节）。
	//
	// binding 的 max=50 按字符计数，25 个汉字的密码（75 字节）能通过校验，却在
	// bcrypt.GenerateFromPassword 处得到未登记的 ErrPasswordTooLong——用户看到的
	// 是 C0500「注册失败」。把这条约束收进模块并登记业务码（A0120），才既挡住
	// 非法输入又给出一句人话。见 backend/service/credentials.go。
	ErrPasswordInvalid = errors.New("密码过长，最多 72 字节（约 24 个汉字）")
	ErrLoginExpired    = errors.New("登录已过期，请重新登录")
	ErrCodeInvalid     = errors.New("验证码错误或已过期")
	ErrAttemptTooMany  = errors.New("尝试次数超限")
	ErrMailSendFailed  = errors.New("邮件发送失败")

	// ErrInvalidImage 上传的图片不符合头像约束。约束本身（扩展名白名单与大小上限）
	// 由 pkg/upload 定义与执行（ADR-0011），这里只承载「服务端拒收这张图」这一业务事实，
	// 好让响应码表在 errcode 一处登记：能力包不该反向成为错误映射的依赖。
	ErrInvalidImage = errors.New("仅支持 jpg/png/webp 图片，且大小不超过 5MB")
)
