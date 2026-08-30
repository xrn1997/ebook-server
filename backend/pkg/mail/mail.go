// Package mail 提供验证码邮件发送的 adapter：SMTP 真实发送与写日志降级。
//
// 选择哪个 adapter 由 main.go 按配置装配，本包不读取任何全局配置（ADR-0007）。
// 两个类型都满足 service 包定义的 Mailer 接口。
package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/smtp"

	"ebook-server/config"
	"ebook-server/pkg/logger"

	"go.uber.org/zap"
)

// SMTPMailer 通过 SMTP 发送验证码邮件（465 端口走显式 TLS，其余走 STARTTLS）。
type SMTPMailer struct {
	cfg config.SMTPConfig
}

// NewSMTPMailer 创建 SMTP 邮件 adapter。
func NewSMTPMailer(cfg config.SMTPConfig) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

// SendCode 发送验证码邮件。
//
// SMTP 未配置时返回错误：release 模式装配本 adapter 即代表「不允许把验证码
// 写进日志」，此时发送失败必须显式暴露，而不是悄悄降级。
func (m *SMTPMailer) SendCode(to, codeVal string) error {
	if m.cfg.Host == "" || m.cfg.Port == 0 {
		return errors.New("SMTP 未配置，无法发送验证码")
	}
	return m.send(to, "ebook 验证码", "您的验证码是："+codeVal+"，5 分钟内有效。")
}

// send 通过 SMTP 发送纯文本邮件
func (m *SMTPMailer) send(to, subject, body string) error {
	cfg := m.cfg
	from := cfg.From
	if from == "" {
		from = cfg.Username
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// 465 端口走显式 TLS
	if cfg.Port == 465 {
		return m.sendTLS(addr, from, to, msg)
	}

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

func (m *SMTPMailer) sendTLS(addr, from, to, msg string) error {
	cfg := m.cfg
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: cfg.Insecure})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	return w.Close()
}

// LogMailer 本地联调用降级 adapter：把验证码写入日志而不是真实发送。
type LogMailer struct{}

// NewLogMailer 创建写日志 adapter。
func NewLogMailer() *LogMailer { return &LogMailer{} }

// SendCode 将验证码写入日志并视为发送成功。
func (m *LogMailer) SendCode(to, codeVal string) error {
	logger.Info("SMTP 未配置（debug），验证码写入日志", zap.String("to", to), zap.String("code", codeVal))
	return nil
}
