package config

import (
	"reflect"
	"strings"
	"testing"
)

// TestEveryConfigKeyIsRegisteredForViper Config 的每个叶子字段都必须登记进 fieldDefaults。
//
// 反向走结构体而不是对照一份手写清单：「新加字段漏写 mapstructure 标签」和
// 「有标签但没登记默认值/环境变量」都会让 viper 安静地读不到配置——显式 YAML
// 可能仍有值，`BindEnv`/`SetDefault` 却没有这个键，环境变量覆盖直接失效。
// 手写清单只会跟着结构体一起漏掉同一个字段，拦不住任何东西。
func TestEveryConfigKeyIsRegisteredForViper(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range fieldDefaults {
		seen[d.Section+"."+d.Key] = true
	}

	cfgType := reflect.TypeOf(Config{})
	for i := 0; i < cfgType.NumField(); i++ {
		section := cfgType.Field(i)
		sectionName, ok := section.Tag.Lookup("mapstructure")
		if !ok || section.Type.Kind() != reflect.Struct {
			t.Errorf("配置段 %s 缺 mapstructure 标签或不是结构体：整段都不会进配置契约", section.Name)
			continue
		}
		for j := 0; j < section.Type.NumField(); j++ {
			leaf := section.Type.Field(j)
			key, ok := leaf.Tag.Lookup("mapstructure")
			if !ok {
				t.Errorf("%s.%s 缺 mapstructure 标签：viper 不会读取它", sectionName, leaf.Name)
				continue
			}
			if !seen[sectionName+"."+key] {
				t.Errorf("%s.%s 未登记进 fieldDefaults：环境变量覆盖和默认值都不会生效", sectionName, key)
			}
		}
	}
}

// TestValidationRulesRejectInvalidValues 规则表覆盖端口、空密钥与后台监听地址。
func TestValidationRulesRejectInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"端口越界", &Config{Server: ServerConfig{Port: 70000}}, "server.port"},
		{"运行模式非法", &Config{Server: ServerConfig{Mode: "prod"}}, "server.mode"},
		{"用户 token 有效期为零", &Config{JWT: JWTConfig{ExpireMin: 0}}, "jwt.expire_min"},
		{"后台 token 有效期越界", &Config{Admin: AdminConfig{ExpireMin: 700000}}, "admin.expire_min"},
		{"后台端口越界", &Config{Admin: AdminConfig{ListenPort: 0}}, "admin.listen_port"},
		{"通配监听", &Config{Admin: AdminConfig{ListenAddr: "0.0.0.0"}}, "通配地址"},
		{"未指定 IPv6（短写法）", &Config{Admin: AdminConfig{ListenAddr: "::0"}}, "通配地址"},
		{"未指定 IPv6（全零写法）", &Config{Admin: AdminConfig{ListenAddr: "0:0:0:0:0:0:0:0"}}, "通配地址"},
		{"主机名监听", &Config{Admin: AdminConfig{ListenAddr: "my-host.local"}}, "IP 地址或 localhost"},
		{"头像来源带路径", &Config{Server: ServerConfig{PublicBaseURL: "https://book.example.com/uploads"}}, "server.public_base_url"},
		{"头像来源缺 scheme", &Config{Server: ServerConfig{PublicBaseURL: "book.example.com"}}, "server.public_base_url"},
		{"头像来源用了非 http 协议", &Config{Server: ServerConfig{PublicBaseURL: "ftp://book.example.com"}}, "server.public_base_url"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.cfg.JWT.Secret = "jwt"
			c.cfg.Admin.JWTSecret = "admin-jwt"
			c.cfg.Admin.Username = "admin"
			c.cfg.Upload.Dir = "uploads"
			err := c.cfg.validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("validate() = %v, want contains %q", err, c.want)
			}
		})
	}
}

// TestPublicBaseURLAcceptsOrigins 来源只要求「一个 origin」：留空（按请求 Host 拼接）、
// 或带 scheme 的主机（可带端口）。带路径/查询的那类值容易被误当成前缀，规则表拒掉。
func TestPublicBaseURLAcceptsOrigins(t *testing.T) {
	for _, origin := range []string{
		"",
		"http://127.0.0.1:9090",
		"https://book.example.com",
		"https://book.example.com:8443",
		// 单个尾斜杠是同一个 origin；upload.New 也按带不带尾斜杠给出同一个 URL。
		"https://book.example.com/",
	} {
		cfg := &Config{
			Server: ServerConfig{Port: 9090, Mode: "release", PublicBaseURL: origin},
			JWT:    JWTConfig{Secret: "jwt", ExpireMin: 60},
			Admin:  AdminConfig{JWTSecret: "admin-jwt", Username: "admin", ExpireMin: 60, ListenAddr: "127.0.0.1", ListenPort: 9091},
			Upload: UploadConfig{Dir: "uploads"},
		}
		if err := cfg.validate(); err != nil {
			t.Errorf("public_base_url = %q 应通过校验，got %v", origin, err)
		}
	}
}
