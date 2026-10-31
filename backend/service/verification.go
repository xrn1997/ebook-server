package service

import (
	"context"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/ratelimit"
)

// Flow 验证码所属的流程。
//
// 一个流程对应一条独立的发码配额（限流键）与独立的码存储命名空间（`":"` 前缀），
// 二者同源（见 VerificationCodes）。新增流程只需加一枚举值 + 一处前缀映射。
type Flow string

const (
	FlowRegister Flow = "reg"
	FlowForgot   Flow = "forgot"
	FlowDeletion Flow = "del"
)

// prefix 返回该流程的码存储命名空间前缀（同时用作限流配额键前缀）。
//
// 存储键 = 限流键 = 流程前缀 + 邮箱，这条约定只在本模块内可见：Send 与 Confirm
// 同源取键，调用方无从（也无需）自己拼一遍——历史上方程式由各调用点自拼，
// 改键格式要找齐四处。
func (f Flow) prefix() string { return string(f) + ":" }

// VerificationCodes 验证码纵深模块（ADR-0008）：拥有「下发」与「校验」两个动词。
//
// 下发：流程化命名空间键 → 双窗口限流 → 存码 → 派发邮件（Mailer）。
// 校验：同一把键 → 把 pkg/code 的四值结果翻译成领域错误。
// 两半住在同一个 module，存储键的拼法只有这里知道；TTL 与尝试上限仍由 pkg/code 持有，
// 但生产代码里只有 Confirm 一处解释那四值结果，调用方只递 flow+邮箱(+码)并接一条领域错误。
//
// 结果枚举外泄过一次代价：三个校验点各自解释结果时，注册流程把「尝试超限」折成
// 「验证码错误」，A0241 因此在 /api/auth/register 上永不可达。判定收进 Confirm 后，
// 三条路径共用同一处翻译——那是这段历史留在这里的理由，不是现状。
//
// 对账号无感（ADR-0008 §4）：账号存在性校验与防枚举是流程自身的策略（ADR-0006），
// 留在调用方。
//
// 依赖以具体类型注入（ADR-0007：第二适配器出现前不立 seam）。code.Store 与 Mailer 由
// main.go 装配唯一实例，再注入各流程模块。
type VerificationCodes struct {
	codes  *code.Store
	mailer Mailer

	minute *ratelimit.Limiter // 发码限流：每分钟至多 1 次
	hour   *ratelimit.Limiter // 发码限流：每小时至多 5 次
}

// NewVerificationCodes 创建验证码模块实例。
func NewVerificationCodes(codes *code.Store, mailer Mailer) *VerificationCodes {
	minute, hour := newSendCodeLimiters()
	return &VerificationCodes{
		codes:  codes,
		mailer: mailer,
		minute: minute,
		hour:   hour,
	}
}

// Send 给指定流程的邮箱下发一个验证码。
//
// 同一配额键（flow+email）在发码限流内才放行，超限返回 model.ErrAttemptTooMany。
// 下发的码只能由同一 flow 的 Confirm 消费：流程之间互不通用。
// ctx 目前只用于调用方签名一致；内存存储与 Mailer 没有取消路径（ADR-0022）。
func (s *VerificationCodes) Send(ctx context.Context, flow Flow, email string) error {
	key := flow.prefix() + email

	// 双窗口限流：两道任一超限即拒绝（ADR-0002 §4）
	if !s.minute.Allow(key) || !s.hour.Allow(key) {
		return model.ErrAttemptTooMany
	}

	codeVal := s.codes.Save(key)
	if err := s.mailer.SendCode(email, codeVal); err != nil {
		// 只回领域哨兵，不追加 err 的文本：邮件通道坏了与服务器自己坏了不是一回事，
		// 客户端要听到两个业务码（C0503 / C0500），而统一信封的文案就是这句话。
		// SMTP 的原始错误由 pkg/mail 在发信那一刻写进日志——细节留在服务端。
		return model.ErrMailSendFailed
	}
	return nil
}

// Confirm 校验某流程下发给该邮箱的验证码。
//
// 校验通过返回 nil 并消费该码（一次性）；尝试次数超限返回 model.ErrAttemptTooMany
// （A0241）；无有效码与码不符统一返回 model.ErrCodeInvalid（A0132）——两者对客户端
// 是同一句话，分开回反而泄露「有没有码」。
func (s *VerificationCodes) Confirm(ctx context.Context, flow Flow, email, codeVal string) error {
	switch s.codes.Verify(flow.prefix()+email, codeVal) {
	case code.ResultOK:
		return nil
	case code.ResultTooManyAttempts:
		return model.ErrAttemptTooMany
	default:
		return model.ErrCodeInvalid
	}
}

// newSendCodeLimiters 创建一对新的发码限流器（每分钟 1 次、每小时 5 次）。
func newSendCodeLimiters() (minute, hour *ratelimit.Limiter) {
	return ratelimit.New(1, time.Minute), ratelimit.New(5, time.Hour)
}
