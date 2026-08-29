package mail

import (
	"crypto/tls"
	"fmt"
	"net/smtp"

	"ebook-server/config"
	"ebook-server/pkg/logger"

	"go.uber.org/zap"
)

// SendCode 发送验证码邮件。
//
// SMTP 未配置时：debug 模式退回将验证码写入日志（便于本地联调），release 模式
// 直接报错，避免在正规环境把验证码悄悄漏进日志。
func SendCode(to, code string) error {
	cfg := config.AppConfig.SMTP
	if cfg.Host == "" || cfg.Port == 0 {
		if config.AppConfig.Server.Mode == "release" {
			return fmt.Errorf("SMTP 未配置，无法发送验证码")
		}
		logger.Info("SMTP 未配置（debug），验证码写入日志", zap.String("to", to), zap.String("code", code))
		return nil
	}
	return Send(to, "ebook 验证码", "您的验证码是："+code+"，5 分钟内有效。")
}

// Send 通过 SMTP 发送纯文本邮件
func Send(to, subject, body string) error {
	cfg := config.AppConfig.SMTP
	from := cfg.From
	if from == "" {
		from = cfg.Username
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// 465 端口走显式 TLS
	if cfg.Port == 465 {
		return sendTLS(addr, cfg, from, to, msg)
	}

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

func sendTLS(addr string, cfg config.SMTPConfig, from, to, msg string) error {
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
