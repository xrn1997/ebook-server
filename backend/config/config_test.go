package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// writeTempYaml 生成一份临时 config.yaml，用于验证「环境变量覆盖 YAML」这条契约。
// sensitiveKeys 控制 smtp/admin 段是否保留 password 字段——桌面应用会把敏感字段
// 从 config.yaml 拆走只留在 .env 里，正是要复现的那种形态。
func writeTempYaml(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	return path
}

// TestLoadConfigSmtpPasswordFromEnv 验证 SMTP_PASSWORD 能覆盖配置。
// 回归场景：桌面应用保存配置后 config.yaml 内不存在 smtp.password 键，
// 若该键未注册进 viper 的已知键集合，Unmarshal 不会查环境变量，密码恒为空。
func TestLoadConfigSmtpPasswordFromEnv(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
database:
  path: ebook.db
smtp:
  host: smtp.example.com
  port: 465
  username: no-reply@example.com
  from: no-reply@example.com
  insecure: false
`)

	t.Setenv("SMTP_PASSWORD", "s3cret-from-env")

	if err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig 失败: %v", err)
	}
	if got := AppConfig.SMTP.Password; got != "s3cret-from-env" {
		t.Errorf("smtp.password 未被 SMTP_PASSWORD 覆盖: got %q, want %q", got, "s3cret-from-env")
	}
}

// TestLoadConfigSmtpPasswordStaysInYaml 验证未拆分（密码仍写在 YAML 里）时读取正常，
// 即纯 YAML 部署方式不因注册默认值而改变行为。
func TestLoadConfigSmtpPasswordStaysInYaml(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
smtp:
  host: smtp.example.com
  password: plain-in-yaml
`)

	if err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig 失败: %v", err)
	}
	if got := AppConfig.SMTP.Password; got != "plain-in-yaml" {
		t.Errorf("smtp.password 读取错误: got %q, want %q", got, "plain-in-yaml")
	}
}

// TestLoadConfigSecretsFromEnv 覆盖同类键的既有契约，防止 smtp 的改动回退掉
// JWT_SECRET / ADMIN_PASSWORD 的覆盖能力（这两个键早有 SetDefault，故一直可用）。
func TestLoadConfigSecretsFromEnv(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
`)

	t.Setenv("JWT_SECRET", "jwt-from-env")
	t.Setenv("ADMIN_PASSWORD", "admin-from-env")
	t.Setenv("ADMIN_JWT_SECRET", "admin-jwt-from-env")

	if err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig 失败: %v", err)
	}
	if AppConfig.JWT.Secret != "jwt-from-env" {
		t.Errorf("JWT_SECRET 未生效: got %q", AppConfig.JWT.Secret)
	}
	if AppConfig.Admin.Password != "admin-from-env" {
		t.Errorf("ADMIN_PASSWORD 未生效: got %q", AppConfig.Admin.Password)
	}
	if AppConfig.Admin.JWTSecret != "admin-jwt-from-env" {
		t.Errorf("ADMIN_JWT_SECRET 未生效: got %q", AppConfig.Admin.JWTSecret)
	}
}
