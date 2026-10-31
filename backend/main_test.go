package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"ebook-server/config"
	"ebook-server/pkg/mail"
)

// TestServe_ReportsListenFailure 监听起不来时 serve 必须把错误回报给调用方。
//
// 调用方（main）靠这条回报走优雅关闭再退出。若 serve 自己 log.Fatalf，进程当场终止：
// 另一个监听还在 Accept 却没人收，在途请求丢失，数据库句柄也不关。
func TestServe_ReportsListenFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	errs := make(chan error, 1)
	serve(&http.Server{Addr: ln.Addr().String()}, "test", errs)

	select {
	case got := <-errs:
		if got == nil {
			t.Fatal("端口被占应回报错误，却收到 nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("监听失败没有回报")
	}
}

// TestServe_ShutdownIsNotAFailure 优雅关闭产生的 http.ErrServerClosed 不是失败：
// 把它回报出去会让每次正常退出都被当成启动错误、多出一个非零退出码。
func TestServe_ShutdownIsNotAFailure(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0"}
	errs := make(chan error, 1)
	go serve(srv, "test", errs)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case got := <-errs:
		t.Fatalf("正常关闭不应回报错误，got %v", got)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestNewMailer_SelectsAdapter 装配层只做一件事：按配置选邮件 adapter。
//
// SMTP 已配置 → 真实发送；未配置且非 release → 写日志（本地联调）；
// release 下未配置也装配 SMTP adapter，让发送时显式报错，避免验证码悄悄漏进日志。
func TestNewMailer_SelectsAdapter(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
		want any
	}{
		{"SMTP 已配置", &config.Config{SMTP: config.SMTPConfig{Host: "smtp.example.com", Port: 465}}, (*mail.SMTPMailer)(nil)},
		{"release 未配置 SMTP", &config.Config{Server: config.ServerConfig{Mode: config.ModeRelease}}, (*mail.SMTPMailer)(nil)},
		{"debug 未配置 SMTP", &config.Config{Server: config.ServerConfig{Mode: "debug"}}, (*mail.LogMailer)(nil)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := newMailer(c.cfg)
			switch c.want.(type) {
			case *mail.SMTPMailer:
				if _, ok := got.(*mail.SMTPMailer); !ok {
					t.Fatalf("got %T, want *mail.SMTPMailer", got)
				}
			case *mail.LogMailer:
				if _, ok := got.(*mail.LogMailer); !ok {
					t.Fatalf("got %T, want *mail.LogMailer", got)
				}
			}
		})
	}
}
