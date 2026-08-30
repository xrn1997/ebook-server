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
	Admin    AdminConfig    `mapstructure:"admin"`    // 后台管理系统（ADR-0009）
	Upload   UploadConfig   `mapstructure:"upload"`   // 文件上传（ADR-0011）
	APIDocs  APIDocsConfig  `mapstructure:"api_docs"` // Swagger 文档开关
}

// APIDocsConfig Swagger API 文档配置。
//
// Swagger UI / OpenAPI spec 会暴露完整接口清单与参数，属于侦察信息。
// 公开 API 端口（0.0.0.0）默认**不**提供文档（enabled=false，防信息泄露）；
// 后台引擎（默认仅 127.0.0.1）不受此开关影响、始终提供，供本机管理查阅。
type APIDocsConfig struct {
	// Enabled 是否在公开 API 端口提供 Swagger 文档（联调/内网开放时置 true）。
	Enabled bool `mapstructure:"enabled"`
}

// UploadConfig 文件上传配置（ADR-0011）。
type UploadConfig struct {
	// Dir 上传文件根目录（相对或绝对路径）。头像存于 <dir>/avatar/，
	// 经 /uploads/* 公开访问（仅存头像等公开资源，不存私有文件）。
	Dir string `mapstructure:"dir"`
}

// AdminConfig 后台管理系统配置（ADR-0009）。密码/密钥经 .env 覆盖。
type AdminConfig struct {
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	JWTSecret string `mapstructure:"jwt_secret"`
	ExpireMin int    `mapstructure:"expire_min"` // 管理端 token 有效期（分钟）
	// ListenAddr 后台监听地址：默认 127.0.0.1（仅本机可访问，公网物理隔离）；
	// 需局域网内其他机器访问时改为内网 IP（如 192.168.1.10）。与公开 API 分开监听。
	ListenAddr string `mapstructure:"listen_addr"`
	// ListenPort 后台监听端口，与公开 API 端口分离（默认 9091）。
	ListenPort int `mapstructure:"listen_port"`
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

	// 后台管理系统默认值（ADR-0009）；部署时需通过 ADMIN_PASSWORD / ADMIN_JWT_SECRET 覆盖。
	// 后台默认仅监听本机回环地址（127.0.0.1:9091），与公开 API 网络隔离，公网无法直接访问。
	viper.SetDefault("admin.username", "admin")
	viper.SetDefault("admin.password", "admin123-dev")
	viper.SetDefault("admin.jwt_secret", "ebook-admin-secret-key")
	viper.SetDefault("admin.expire_min", 60)
	viper.SetDefault("admin.listen_addr", "127.0.0.1")
	viper.SetDefault("admin.listen_port", 9091)

	// 文件上传（ADR-0011）：默认存仓库根下 uploads/ 目录
	viper.SetDefault("upload.dir", "uploads")

	// Swagger 文档：公开端口默认不提供（防信息泄露），联调/内网开放时置 true
	viper.SetDefault("api_docs.enabled", false)

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}
