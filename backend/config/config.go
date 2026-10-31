// Package config 提供应用配置加载与管理，支持 YAML 文件 + .env 环境变量覆盖。
package config

import (
	"errors"
	"fmt"
	"os"
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
	// 仅该子目录经 /uploads/avatar 公开访问（根目录下同处还放着配置文件与数据库，
	// 整目录挂出去就是把它们公开）。
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
	// PublicBaseURL 是公开资源（头像）绝对 URL 的来源，形如 https://book.example.com。
	//
	// 留空时按请求 Host 拼接（自托管没有固定域名时的便利，ADR-0011），代价是伪造 Host
	// 能把任意外链写进数据库；对外提供服务时应钉死这一项。规则表只接受纯 origin。
	PublicBaseURL string `mapstructure:"public_base_url"`
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

// 配置不设包级全局：全局实例会把「必须先有人调用加载」写成每个使用方的隐式前提，
// 并且让测试只能通过改写全局、串行执行。main.go 加载一次，按值注入各模块。

// Load 从指定路径加载 YAML 配置，并自动读取 .env（若存在）。
// 环境变量可覆盖 YAML 中的同名键（如 SMTP_PASSWORD → smtp.password）。
//
// 用独立的 viper 实例而非包级默认实例：后者让两次 Load 互相污染，测试无法并行。
func Load(path string) (*Config, error) {
	// 加载 .env（存在才生效），使敏感配置可通过环境变量覆盖（如 SMTP_PASSWORD / JWT_SECRET）。
	// 只忽略「文件不存在」：.env 存在但语法错误时必须当场失败——静默跳过会让 release
	// 模式把「密钥没读到、仍用内置默认值」报成一句与真正原因无关的话，debug 模式则
	// 直接带着内置密钥跑起来。
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load .env: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	// 开启自动读环境变量：viper 会把键转成大写并把分隔符替换成下划线
	// （server.port → SERVER_PORT）。敏感项仍显式绑定文档承诺的名字（JWT_SECRET 等）：
	// Unmarshal 只遍历已知键，而显式绑定让「文档名 → 配置键」不依赖 viper 的内部转换规则。
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ModeRelease 生产运行模式。与 gin.ReleaseMode 同值，此处不 import gin：
// 「配置处于哪种模式」不该依赖 Web 框架，而 main.go 判 SMTP 时也要用同一个字面量。
const ModeRelease = "release"

// validate 拒绝那些会让认证安全或网络隔离形同虚设的配置。
//
// 规则表在 schema.go（validationRules），启动校验按同一份文案拒绝：
//   - 空 jwt.secret 是最容易踩到、后果又最重的一种：密钥为空时 HMAC 仍然「工作」，
//     于是任何客户端都能自行签出任意 uid 的 access token，而公开 API 绑在 0.0.0.0。
//     它偏偏又极易发生——配置文件里的显式空值优先于 SetDefault，默认值兜不住它，
//     而把密钥留空、改由 `.env` 提供正是常见写法。
//   - 空 upload.dir 在 net/http 里等于进程当前目录，会把同目录下的 config.yaml、
//     .env 与数据库一并公开到 /uploads 之下。
//   - 后台监听通配地址会撕开 ADR-0010 的网络隔离。
//
// 拦在这里而不是各调用点：Load 是配置进入进程的唯一闸口，放行与否不该取决于
// 谁记得检查。内置默认值只在 release 下拒绝，本地开发的零配置启动不受影响。
func (c *Config) validate() error {
	problems := validateRules(c)
	if c.Server.Mode == ModeRelease {
		for _, weak := range c.builtInSecrets() {
			problems = append(problems, weak+" 仍是代码内置的默认值")
		}
		// 仓库里公开的占位值和内置默认值是同一类风险，只是来源不同：前者是
		// .env.example / config.yaml 的样板串，后者是源码里的常量。两者在
		// release 下都不能继续用——复制 .env.example 不改内容就上线，等于用一把
		// 全网读得到的密钥签名。
		for _, weak := range placeholderSecrets(fieldValues(c)) {
			problems = append(problems, weak+" 仍是仓库里公开的占位值")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("配置校验失败：%s。请修正 config.yaml；密钥类配置也可用环境变量提供"+
			"（JWT_SECRET / ADMIN_JWT_SECRET / ADMIN_PASSWORD）", strings.Join(problems, "、"))
	}
	return nil
}

// builtInSecrets 列出当前仍在使用代码内置值的配置项（返回字段名，供拼错误消息）。
//
// 哪些字段算内置凭据由 fieldDefaults 的 BuiltInSecret 标记定义，这里不再维护
// 第二份字段清单。
func (c *Config) builtInSecrets() []string {
	values := fieldValues(c)
	var weak []string
	for _, d := range fieldDefaults {
		if d.BuiltInSecret && values[d.Section+"."+d.Key] == d.Value {
			weak = append(weak, d.Section+"."+d.Key)
		}
	}
	return weak
}

// setDefaults 按 schema.go 的注册表登记默认值与环境变量覆盖。
//
// 这里只剩执行逻辑：加一个默认值或环境变量键只改 fieldDefaults，两处不再各写一份。
//
// 注意 smtp.password 这条看似多余的默认值：viper 的 Unmarshal 只遍历它「已知」的键
// （默认值 / 配置文件 / BindEnv），未注册的键即使开了 AutomaticEnv 也不会被查到——
// 注册它是为了让该键始终有个可覆盖的落点（SMTP 口令可从 `.env` 提供）。
// 登记了 Env 的键还会显式 BindEnv，见下面的说明。
func setDefaults(v *viper.Viper) {
	for _, d := range fieldDefaults {
		key := d.Section + "." + d.Key
		// Value 为 nil 表示「登记这个键但不给它默认值」（database.path 的缺省文件名
		// 由 pkg/database.DefaultPath 决定）。SetDefault 仍要调：它把键放进 viper 的
		// 已知键集合，配置文件里没有那一段时环境变量覆盖才不会被整段跳过。
		v.SetDefault(key, d.Value)

		// 敏感项的环境变量覆盖必须**显式绑定文档承诺的大写名**。AutomaticEnv 当前也会
		// 把 `jwt.secret` 转成 `JWT_SECRET` 去查，但那是 viper 的内部转换规则，不是本仓的
		// 契约；Unmarshal 又只遍历已知键（默认值 / 配置文件 / BindEnv）。显式绑定把
		// 「文档名 → 配置键」写死在这里，换版本或改前缀时不会静默落回内置默认密钥。
		// 其余键交给 AutomaticEnv 兜底。
		if d.Env != "" {
			_ = v.BindEnv(key, d.Env)
		}
	}
}
