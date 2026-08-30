package service

import (
	"testing"

	"ebook-server/model"
	"ebook-server/pkg/code"
)

// recordingMailer 记录最近一次 SendCode 调用，便于断言派发目标与内容。
type recordingMailer struct {
	to      string
	codeVal string
}

func (m *recordingMailer) SendCode(to, codeVal string) error {
	m.to = to
	m.codeVal = codeVal
	return nil
}

// TestVerificationCodeSender_Send_StoresAndDispatches 发码应落进对应流程命名空间并派发邮件
func TestVerificationCodeSender_Send_StoresAndDispatches(t *testing.T) {
	codes := code.NewStore()
	mailer := &recordingMailer{}
	sender := NewVerificationCodeSender(codes, mailer)

	if err := sender.Send(FlowRegister, "v@example.com"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// 码应落在 "reg:" 命名空间（流程隔离）
	if got := codes.Verify(FlowRegister.prefix()+"v@example.com", ""); got == code.ResultNotFound {
		t.Error("Expected a stored code under FlowRegister namespace")
	}
	if mailer.to != "v@example.com" {
		t.Errorf("Expected mail to 'v@example.com', got %q", mailer.to)
	}
	if mailer.codeVal == "" {
		t.Error("Expected a non-empty code dispatched")
	}
}

// TestVerificationCodeSender_Send_RateLimit_IsolatedPerFlow 限流按流程+邮箱隔离，互不占配额
func TestVerificationCodeSender_Send_RateLimit_IsolatedPerFlow(t *testing.T) {
	codes := code.NewStore()
	sender := NewVerificationCodeSender(codes, &recordingMailer{})

	// 同一流程同一邮箱：首发成功，二次（窗口内）被限流
	if err := sender.Send(FlowRegister, "v@example.com"); err != nil {
		t.Fatalf("First send should succeed: %v", err)
	}
	if err := sender.Send(FlowRegister, "v@example.com"); err != model.ErrAttemptTooMany {
		t.Fatalf("Second send should hit rate limit, got %v", err)
	}

	// 不同流程：配额独立，仍可发
	if err := sender.Send(FlowForgot, "v@example.com"); err != nil {
		t.Fatalf("Forgot flow quota should be independent: %v", err)
	}
}
