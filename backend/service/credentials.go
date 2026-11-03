package service

import (
	"context"
	"errors"
	"time"

	"ebook-server/model"

	"golang.org/x/crypto/bcrypt"
)

// 登录防暴力锁定参数（ADR-0002）。策略归凭据模块：失败计数与锁定时长在同一处定义。
const (
	maxLoginAttempts  = 5
	loginLockDuration = 15 * time.Minute
)

// loginHashPlaceholder 一枚从不使用的占位摘要：随机口令取完哈希即丢弃，无人知道它。
//
// 它只服务于「邮箱没注册时也要花掉一次 bcrypt 的代价」（ADR-0006 的时序一侧）。
// 写死而不是启动时生成：每次进程启动都跑一遍哈希没有意义，代价却落在冷启动上。
const loginHashPlaceholder = "$2a$10$HS7moy/oOUiT4C7sNcetju2f/0OFL9f3nPGbVBo6cufC1pwP7XNou"

// maxPasswordBytes 是 bcrypt 的硬上限：超过 72 字节的输入，GenerateFromPassword
// 直接返回 ErrPasswordTooLong（自 bcrypt 的 2^8 位密钥调度，72 字节是能进 key schedule
// 的长度）。请求结构体的 binding 用 max=50 按**字符**计数，25 个汉字的密码就有 75 字节：
// 能过校验进入模块，哈希时失败，而未登记的 ErrPasswordTooLong 会落成 C0500。
// 因此在模块边界按字节再判一次，并由 errcode 表映射成 A0120。
const maxPasswordBytes = 72

// checkPasswordLength 在写库/哈希之前拒绝超出 bcrypt 上限的明文。
func checkPasswordLength(plain string) error {
	if len(plain) > maxPasswordBytes {
		return model.ErrPasswordInvalid
	}
	return nil
}

// Credentials 密码与登录失败策略的唯一实现。
//
// 改密（Change）与重置（Reset）的编排入口在这里：验旧口令（仅改密）、撤销会话、
// 写新哈希。共用的 setPassword 先撤销会话（refresh token）、再写新哈希：撤销失败则密码不变；
// 写哈希失败则用户被登出但旧密码仍可用。两种失败都不会留下
// 「新密码生效、旧 refresh token 仍活着」的状态（fail-closed）。
type Credentials struct {
	users CredentialStore
	// sessions 是会话撤销的唯一入口（Sessions.Revoke）：改密撤销与登出、
	// 注销走同一条路径，规则不各自复制一份。
	sessions *Sessions
	// compare 是口令摘要比对单元，构造时固定为 bcrypt.CompareHashAndPassword。
	// 生产路径不覆写它；覆写方只有 auth_test 的两条时序等价用例（未知邮箱比对占位
	// 摘要、锁定期内仍比对），它们从这道缝上数比对的次数与入参，而不是靠掐表测时。
	// 它不是换算法的接缝：换算法走 NewCredentials 这个装配点。
	compare func(hashed, plain []byte) error
}

// NewCredentials 创建凭据服务实例。
func NewCredentials(users CredentialStore, sessions *Sessions) *Credentials {
	return &Credentials{
		users:    users,
		sessions: sessions,
		compare:  bcrypt.CompareHashAndPassword,
	}
}

// CreateWithPassword 哈希并落库新账号；注册是唯一创建凭据的入口。
func (c *Credentials) CreateWithPassword(ctx context.Context, user *model.User, plain string) error {
	if err := checkPasswordLength(plain); err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hashed)
	return c.users.Create(ctx, user)
}

// checkPassword 校验明文密码；只返回 bool，业务错误由调用方决定。纯计算，不需要 ctx。
// 包内未导出：对外的口令校验入口是 Verify（登录）与 Change（改密），不单独暴露比对原语。
func (c *Credentials) checkPassword(hash, plain string) bool {
	return c.compare([]byte(hash), []byte(plain)) == nil
}

// Verify 登录专用的等代价口令校验。
//
// user 为 nil（邮箱没注册）或账号已匿名化到没有口令摘要时，比对落到占位摘要上：
// 结果必然为 false，但耗时与真实账号一致。「这个邮箱注册过吗」于是既问不出在
// 响应码里，也问不出在响应时间里（ADR-0006）。
func (c *Credentials) Verify(user *model.User, plain string) bool {
	hash := loginHashPlaceholder
	if user != nil && user.Password != "" {
		hash = user.Password
	}
	return c.checkPassword(hash, plain)
}

// setPassword 更换密码并撤销全部会话（改密/重置共用）。
func (c *Credentials) setPassword(ctx context.Context, uid uint, plain string) error {
	if err := checkPasswordLength(plain); err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := c.sessions.Revoke(ctx, uid); err != nil {
		return err
	}
	return c.users.UpdatePasswordHash(ctx, uid, string(hashed))
}

// Change 已登录改密的编排：读账号 → 验旧口令 → 撤销全部会话 → 写新哈希。
//
// 撤销失败则中止、不写新哈希（fail-closed，由 setPassword 的顺序承担）：
// 否则会出现「新口令生效、旧 refresh token 仍活着」的中间态。
// 旧口令比对失败回 model.ErrPasswordWrong——改密端点处于已认证上下文，
// 不参与登录面的响应折叠（IsLoginDenied 只收登录拒绝）。
func (c *Credentials) Change(ctx context.Context, uid uint, oldPassword, newPassword string) error {
	user, err := c.users.FindByUID(ctx, uid)
	if err != nil {
		return err
	}
	if !c.checkPassword(user.Password, oldPassword) {
		return model.ErrPasswordWrong
	}
	return c.setPassword(ctx, uid, newPassword)
}

// Reset 无旧口令的重置编排（找回密码流程，验证码校验留在认证模块）：
// 撤销全部会话 → 写新哈希，同样 fail-closed。
func (c *Credentials) Reset(ctx context.Context, uid uint, newPassword string) error {
	return c.setPassword(ctx, uid, newPassword)
}

// IsLoginDenied 登录拒绝的折叠规则（ADR-0006）：这四枚哨兵在对外响应里必须完全一致。
//
// 折叠判据住在凭据模块，而不是 handler 逐枚列举 model 哨兵：尝试超限与账号锁定
// 只可能出现在已注册账号上，映射时漏掉任何一枚，五次错误密码就能把
// 「这个邮箱注册过吗」问出来。handler 只判一次本函数，命中即同一业务码与文案。
func IsLoginDenied(err error) bool {
	return errors.Is(err, model.ErrAccountNotFound) ||
		errors.Is(err, model.ErrPasswordWrong) ||
		errors.Is(err, model.ErrAttemptTooMany) ||
		errors.Is(err, model.ErrAccountLocked)
}

// RecordFailedLogin 累加一次失败并报告结果。
//
// 「达到阈值」的判定交给存储层那条 UPDATE：Go 这边先递增再读回比大小，读到的
// 可能是并发另一个失败请求的计数，也可能是刚被成功登录清零的值。
func (c *Credentials) RecordFailedLogin(ctx context.Context, uid uint) (bool, error) {
	return c.users.RecordFailedLogin(ctx, uid, maxLoginAttempts, time.Now().Add(loginLockDuration))
}

// ClearFailures 登录成功后清零失败计数与锁定。
func (c *Credentials) ClearFailures(ctx context.Context, uid uint) error {
	return c.users.ClearLoginFailures(ctx, uid)
}
