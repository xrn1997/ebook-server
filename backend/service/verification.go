package service

import (
	"time"

	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/ratelimit"
)

// Flow 验证码所属的流程。
//
// 一个流程对应一条独立的发码配额（限流键）与独立的码存储命名空间（`":"` 前缀），
// 二者同源（见 VerificationCodeSender）。新增流程只需加一枚举值 + 一处前缀映射。
type Flow string

const (
	FlowRegister Flow = "reg"
	FlowForgot   Flow = "forgot"
	FlowDeletion Flow = "del"
)

// prefix 返回该流程的码存储命名空间前缀（同时用作限流配额键前缀）。
//
// 存储键 = 限流键 = 流程前缀 + 邮箱，这条约定集中在本符号表，一处可改。
func (f Flow) prefix() string { return string(f) + ":" }

// VerificationCodeSender 验证码发送纵深模块（ADR-0008）。
//
// 收拢「下发验证码」的完整编排：流程化命名空间键 → 双窗口限流 → 存码 → 派发邮件（Mailer）。
// 限流器随本模块实例归属、按 flow+email 隔离配额；对账号无感——账号存在性校验与防枚举
// 是流程自身的策略（ADR-0006），留在调用方。
//
// 依赖以具体类型注入（ADR-0007：第二适配器出现前不立 seam）。code.Store 与 Mailer 由
// main.go 装配唯一实例，再注入各流程模块。
type VerificationCodeSender struct {
	codes  *code.Store
	mailer Mailer

	minute *ratelimit.Limiter // 发码限流：每分钟至多 1 次
	hour   *ratelimit.Limiter // 发码限流：每小时至多 5 次
}

// NewVerificationCodeSender 创建验证码发送模块实例。
func NewVerificationCodeSender(codes *code.Store, mailer Mailer) *VerificationCodeSender {
	minute, hour := newSendCodeLimiters()
	return &VerificationCodeSender{
		codes:  codes,
		mailer: mailer,
		minute: minute,
		hour:   hour,
	}
}

// Send 给指定流程的邮箱下发一个验证码。
//
// 同一配额键（flow+email）在发码限流内才放行，超限返回 ErrAttemptTooMany。
// 验证码落在 `<flow>:` 命名空间，供调用方在后续的校验端点读取。
func (s *VerificationCodeSender) Send(flow Flow, email string) error {
	key := flow.prefix() + email

	// 双窗口限流：两道任一超限即拒绝（ADR-0002 §4）
	if !s.minute.Allow(key) || !s.hour.Allow(key) {
		return model.ErrAttemptTooMany
	}

	codeVal := s.codes.Save(key)
	return s.mailer.SendCode(email, codeVal)
}

// newSendCodeLimiters 创建一对新的发码限流器（每分钟 1 次、每小时 5 次）。
func newSendCodeLimiters() (minute, hour *ratelimit.Limiter) {
	return ratelimit.New(1, time.Minute), ratelimit.New(5, time.Hour)
}
