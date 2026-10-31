package service

import (
	"context"
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
// SetPassword 先撤销 refresh token、再写新哈希：撤销失败则密码不变；
// 写哈希失败则用户被登出但旧密码仍可用。两种失败都不会留下
// 「新密码生效、旧 refresh token 仍活着」的状态（fail-closed）。
type Credentials struct {
	users  CredentialStore
	tokens TokenStore
	// compare 是口令摘要比对单元，默认 bcrypt。留出这道缝不是为了换算法，
	// 而是让「不存在账号也走一次等代价比对」能被确定性地断言，而不是靠掐表测时。
	compare func(hashed, plain []byte) error
}

// NewCredentials 创建凭据服务实例。
func NewCredentials(users CredentialStore, tokens TokenStore) *Credentials {
	return &Credentials{
		users:   users,
		tokens:  tokens,
		compare: bcrypt.CompareHashAndPassword,
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

// CheckPassword 校验明文密码；只返回 bool，业务错误由调用方决定。纯计算，不需要 ctx。
func (c *Credentials) CheckPassword(hash, plain string) bool {
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
	return c.CheckPassword(hash, plain)
}

// SetPassword 更换密码并撤销全部 refresh token（改密/重置共用）。
func (c *Credentials) SetPassword(ctx context.Context, uid uint, plain string) error {
	if err := checkPasswordLength(plain); err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := c.tokens.DeleteByUserID(ctx, uid); err != nil {
		return err
	}
	return c.users.UpdatePasswordHash(ctx, uid, string(hashed))
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
