// Package config 提供应用配置加载与管理，支持 YAML 文件 + .env 环境变量覆盖。
package config

import (
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config 应用全局配置（对应 config.yaml 顶层结构）。
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	SMTP     SMTPConfig     `mapstructure:"smtp"`
	Admin    AdminConfig    `mapstructure:"admin"` // 后台管理系统（ADR-0009）
}

// AdminConfig 后台管理系统配置（ADR-0009）。密码/密钥经 .env 覆盖。
type AdminConfig struct {
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	JWTSecret string `mapstructure:"jwt_secret"`
	ExpireMin int    `mapstructure:"expire_min"` // 管理端 token 有效期（分钟）
}

// ServerConfig 服务端配置。
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"` // debug, release, test
}

// DatabaseConfig 数据库配置。
type DatabaseConfig struct {
	Path string `mapstructure:"path"` // SQLite 数据库文件路径
}

// JWTConfig 认证配置
type JWTConfig struct {
	Secret    string `mapstructure:"secret"`
	ExpireMin int    `mapstructure:"expire_min"` // access token 有效期（分钟），默认 120
}

// SMTPConfig 邮件发送配置
type SMTPConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
	Insecure bool   `mapstructure:"insecure"` // 开发环境关闭 TLS 校验
}

// AppConfig 全局配置实例，由 LoadConfig 初始化后供各层读取。
var AppConfig *Config

// LoadConfig 从指定路径加载 YAML 配置，并自动读取 .env（若存在）。
// 环境变量可覆盖 YAML 中的同名键（如 SMTP_PASSWORD → smtp.password）。
func LoadConfig(path string) error {
	// 加载 .env（存在才生效），使敏感配置可通过环境变量覆盖（如 SMTP_PASSWORD / JWT_SECRET）
	_ = godotenv.Load()

	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")
	// 开启自动读环境变量，将嵌套键 smtp.password 映射为 SMTP_PASSWORD
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// 设置默认值
	viper.SetDefault("server.port", 9090)
	viper.SetDefault("server.mode", "debug")
	viper.SetDefault("database.path", "ebook.db")
	viper.SetDefault("jwt.secret", "ebook-secret-key")
	viper.SetDefault("jwt.expire_min", 120)

	// 后台管理系统默认值（ADR-0009）；部署时需通过 ADMIN_PASSWORD / ADMIN_JWT_SECRET 覆盖
	viper.SetDefault("admin.username", "admin")
	viper.SetDefault("admin.password", "admin123-dev")
	viper.SetDefault("admin.jwt_secret", "ebook-admin-secret-key")
	viper.SetDefault("admin.expire_min", 240)

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}
