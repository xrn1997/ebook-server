package service

import (
	"errors"
	"strings"
	"testing"

	"ebook-server/model"
	"ebook-server/pkg/code"
)

// mailAttempt 一次 SendCode 调用的痕迹
type mailAttempt struct {
	to      string
	codeVal string
}

// recordingMailer 记录每一次 SendCode 调用，便于断言派发目标与内容。
//
// 它是 Mailer 的第三个 adapter（另两个：SMTP、写日志降级），测试经它取回
// 模块下发的码——这是预置验证码的唯一途径（见 seedCode）。
type recordingMailer struct {
	sent []mailAttempt
}

func (m *recordingMailer) SendCode(to, codeVal string) error {
	m.sent = append(m.sent, mailAttempt{to: to, codeVal: codeVal})
	return nil
}

// last 最近一次发给该邮箱的码；从未发过则返回空串。
func (m *recordingMailer) last(email string) string {
	for i := len(m.sent) - 1; i >= 0; i-- {
		if m.sent[i].to == email {
			return m.sent[i].codeVal
		}
	}
	return ""
}

// countTo 发给该邮箱的邮件条数（防枚举断言用：未注册邮箱应为 0）。
func (m *recordingMailer) countTo(email string) int {
	n := 0
	for _, a := range m.sent {
		if a.to == email {
			n++
		}
	}
	return n
}

// newIsolatedModule 造一对全新的（存储 + 记录型 Mailer）模块实例，与测试环境无关。
//
// 模块自身的用例用它：每个 case 都有自己的限流窗口与码存储，互不串扰。
func newIsolatedModule() (*VerificationCodes, *recordingMailer) {
	mailer := &recordingMailer{}
	return NewVerificationCodes(code.NewStore(), mailer), mailer
}

// failingMailer Mailer 的又一个 adapter：邮件通道坏掉的那条路径。
type failingMailer struct{ err error }

func (m failingMailer) SendCode(string, string) error { return m.err }

// TestVerificationCodes_Send_SurfacesMailFailureAsDomainError 邮件失败回 C0503，且不带上 SMTP 细节。
//
// pkg/mail 把 SMTP 错误原样递上来，若不换成领域哨兵，「邮件通道坏了」与「服务器自己
// 坏了」在客户端长成同一个 C0500，而 errcode 表里 ErrMailSendFailed → C0503 那行因为
// 无产生方永远不可达（AGENTS.md 曾把它列为已知问题）。
//
// 第二条断言钉的是反向风险：统一信封的文案取自 err.Error()，把 SMTP 错误拼进消息就等于
// 随 C0503 一起把内网主机与认证失败原因递给未认证调用方。原始错误由 pkg/mail 在发信时
// 写进日志，细节留在服务端。
func TestVerificationCodes_Send_SurfacesMailFailureAsDomainError(t *testing.T) {
	smtpErr := errors.New("dial tcp mail.internal:465: auth failed")
	module := NewVerificationCodes(code.NewStore(), failingMailer{err: smtpErr})

	err := module.Send(testCtx, FlowRegister, "user@example.com")
	if !errors.Is(err, model.ErrMailSendFailed) {
		t.Fatalf("包装链上没有 ErrMailSendFailed，客户端会收到 C0500 而不是 C0503: %v", err)
	}
	if msg := err.Error(); strings.Contains(msg, smtpErr.Error()) {
		t.Errorf("SMTP 原始错误进了响应文案，会随 C0503 一起回给客户端: %q", msg)
	}
}

// TestVerificationCodes_Send_StoresAndDispatches 发码应落进对应流程命名空间并派发邮件
func TestVerificationCodes_Send_StoresAndDispatches(t *testing.T) {
	module, mailer := newIsolatedModule()

	if err := module.Send(testCtx, FlowRegister, "v@example.com"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	codeVal := mailer.last("v@example.com")
	if codeVal == "" {
		t.Fatal("Expected a code dispatched to the address")
	}
	// 下发进 reg: 命名空间的码，由同一流程的 Confirm 消费
	if err := module.Confirm(testCtx, FlowRegister, "v@example.com", codeVal); err != nil {
		t.Errorf("Expected Confirm to accept the dispatched code, got %v", err)
	}
}

// TestVerificationCodes_Send_RateLimit_IsolatedPerFlow 限流按流程+邮箱隔离，互不占配额
func TestVerificationCodes_Send_RateLimit_IsolatedPerFlow(t *testing.T) {
	module, _ := newIsolatedModule()

	// 同一流程同一邮箱：首发成功，二次（窗口内）被限流
	if err := module.Send(testCtx, FlowRegister, "v@example.com"); err != nil {
		t.Fatalf("First send should succeed: %v", err)
	}
	if err := module.Send(testCtx, FlowRegister, "v@example.com"); err != model.ErrAttemptTooMany {
		t.Fatalf("Second send should hit rate limit, got %v", err)
	}

	// 不同流程：配额独立，仍可发
	if err := module.Send(testCtx, FlowForgot, "v@example.com"); err != nil {
		t.Fatalf("Forgot flow quota should be independent: %v", err)
	}
}

// TestVerificationCodes_Confirm_TranslatesResults 校验结果到领域错误的翻译只在这一处
func TestVerificationCodes_Confirm_TranslatesResults(t *testing.T) {
	t.Run("通过的码消费后不可复用", func(t *testing.T) {
		module, mailer := newIsolatedModule()
		if err := module.Send(testCtx, FlowRegister, "a@example.com"); err != nil {
			t.Fatalf("Send failed: %v", err)
		}
		codeVal := mailer.last("a@example.com")

		if err := module.Confirm(testCtx, FlowRegister, "a@example.com", codeVal); err != nil {
			t.Errorf("First Confirm should pass, got %v", err)
		}
		if err := module.Confirm(testCtx, FlowRegister, "a@example.com", codeVal); err != model.ErrCodeInvalid {
			t.Errorf("Replayed code should be consumed, got %v", err)
		}
	})

	t.Run("错码累计到上限回尝试超限而非验证码错误", func(t *testing.T) {
		module, mailer := newIsolatedModule()
		if err := module.Send(testCtx, FlowDeletion, "b@example.com"); err != nil {
			t.Fatalf("Send failed: %v", err)
		}
		wrong := otherCodeThan(mailer.last("b@example.com"))

		for i := 0; i < code.MaxAttempts-1; i++ {
			if err := module.Confirm(testCtx, FlowDeletion, "b@example.com", wrong); err != model.ErrCodeInvalid {
				t.Fatalf("attempt %d: expected ErrCodeInvalid, got %v", i+1, err)
			}
		}
		if err := module.Confirm(testCtx, FlowDeletion, "b@example.com", wrong); err != model.ErrAttemptTooMany {
			t.Errorf("Expected ErrAttemptTooMany at the limit, got %v", err)
		}
	})

	t.Run("无有效码回验证码错误", func(t *testing.T) {
		module, _ := newIsolatedModule()
		if err := module.Confirm(testCtx, FlowForgot, "never-sent@example.com", "123456"); err != model.ErrCodeInvalid {
			t.Errorf("Expected ErrCodeInvalid, got %v", err)
		}
	})
}

// TestVerificationCodes_FlowIsolation 一个流程下发的码不能被另一个流程使用
func TestVerificationCodes_FlowIsolation(t *testing.T) {
	module, mailer := newIsolatedModule()

	if err := module.Send(testCtx, FlowRegister, "iso@example.com"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	codeVal := mailer.last("iso@example.com")

	if err := module.Confirm(testCtx, FlowForgot, "iso@example.com", codeVal); err != model.ErrCodeInvalid {
		t.Errorf("A register code must not confirm the forgot flow, got %v", err)
	}
	if err := module.Confirm(testCtx, FlowRegister, "iso@example.com", codeVal); err != nil {
		t.Errorf("The same code must still confirm its own flow, got %v", err)
	}
}

// otherCodeThan 返回一个必然不等于 given 的 6 位码。
//
// 模块用例先经真下发拿到码、再要一枚「必然错」的码：写死 "000000" 有百万分之一的
// 概率正好命中真码，让用例变成偶发失败。
func otherCodeThan(given string) string {
	if given != "000000" {
		return "000000"
	}
	return "000001"
}
