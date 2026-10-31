package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempYaml 生成一份临时 config.yaml，用于验证「环境变量覆盖 YAML」这条契约。
// sensitiveKeys 控制 smtp/admin 段是否保留 password 字段——密钥通常只放
// .env、config.yaml 里留空或省略，正是要复现的那种形态。
func writeTempYaml(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	return path
}

// TestLoadSmtpPasswordFromEnv 验证 SMTP_PASSWORD 能覆盖配置。
// 回归场景：敏感字段只放 .env、config.yaml 不含 smtp.password 键时，
// 若该键未注册进 viper 的已知键集合，Unmarshal 不会查环境变量，密码恒为空。
func TestLoadSmtpPasswordFromEnv(t *testing.T) {

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

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if got := cfg.SMTP.Password; got != "s3cret-from-env" {
		t.Errorf("smtp.password 未被 SMTP_PASSWORD 覆盖: got %q, want %q", got, "s3cret-from-env")
	}
}

// TestLoadSmtpPasswordStaysInYaml 验证未拆分（密码仍写在 YAML 里）时读取正常，
// 即纯 YAML 部署方式不因注册默认值而改变行为。
func TestLoadSmtpPasswordStaysInYaml(t *testing.T) {

	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
smtp:
  host: smtp.example.com
  port: 465
  password: plain-in-yaml
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if got := cfg.SMTP.Password; got != "plain-in-yaml" {
		t.Errorf("smtp.password 读取错误: got %q, want %q", got, "plain-in-yaml")
	}
}

// TestLoadSecretsFromEnv 覆盖同类键的既有契约，防止 smtp 的改动回退掉
// JWT_SECRET / ADMIN_PASSWORD 的覆盖能力（这两个键早有 SetDefault，故一直可用）。
func TestLoadSecretsFromEnv(t *testing.T) {

	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
`)

	t.Setenv("JWT_SECRET", "jwt-from-env")
	t.Setenv("ADMIN_PASSWORD", "admin-from-env")
	t.Setenv("ADMIN_JWT_SECRET", "admin-jwt-from-env")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.JWT.Secret != "jwt-from-env" {
		t.Errorf("JWT_SECRET 未生效: got %q", cfg.JWT.Secret)
	}
	if cfg.Admin.Password != "admin-from-env" {
		t.Errorf("ADMIN_PASSWORD 未生效: got %q", cfg.Admin.Password)
	}
	if cfg.Admin.JWTSecret != "admin-jwt-from-env" {
		t.Errorf("ADMIN_JWT_SECRET 未生效: got %q", cfg.Admin.JWTSecret)
	}
}

// unsetEnv 让指定环境变量在测试期间真正不存在，结束时恢复原值。
//
// 只调 t.Setenv(key, "") 达不到目的：viper 把「存在但为空」的环境变量也算作已设置，
// 于是被测的其实是另一条路径。先 Setenv 登记恢复钩子、再 Unsetenv 真正删除。
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
}

// TestLoadRejectsEmptyJWTSecret 空密钥必须拒绝启动，而不是悄悄退回默认值。
//
// 复现的是常见配置形态：config.yaml 显式写 secret: ""，而 viper 里
// 「配置文件中的显式值」优先于 SetDefault，默认值兜不住空串。空 HMAC 密钥意味着
// 任何客户端都能自行签发任意 uid 的 access token，而公开 API 绑在 0.0.0.0。
func TestLoadRejectsEmptyJWTSecret(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")

	for _, tc := range []struct{ name, body string }{
		{"用户密钥为空", "jwt:\n  secret: \"\"\nadmin:\n  jwt_secret: table-secret\n"},
		{"后台密钥为空", "jwt:\n  secret: table-secret\nadmin:\n  jwt_secret: \"\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempYaml(t, "server:\n  port: 9090\n  mode: test\n"+tc.body)

			if _, err := Load(path); err == nil {
				t.Fatal("空密钥被接受了：签出的 token 任何客户端都能伪造")
			}
		})
	}
}

// TestEnvSecretOverridesEmptyYamlValue 环境变量必须压过 YAML 里的显式空串。
//
// 这是 .env 覆盖 YAML 的承重点：配置文件写着 secret: ""，真实值只落在 .env。
// 若 viper 让配置文件里的空串赢过环境变量，服务要么带着空密钥上线、
// 要么被上面的 validate 挡在启动之外——两头都不可接受。
func TestEnvSecretOverridesEmptyYamlValue(t *testing.T) {
	path := writeTempYaml(t, `server:
  port: 9090
  mode: release
jwt:
  secret: ""
admin:
  jwt_secret: ""
`)
	t.Setenv("JWT_SECRET", "generated-from-env")
	t.Setenv("ADMIN_JWT_SECRET", "generated-admin-from-env")
	// release 下 admin.password 的内置默认值同样会被拦下（它是一份写在仓库里的口令），
	// 所以这里三份凭据都要给出——与 .env 的实际内容一致。
	t.Setenv("ADMIN_PASSWORD", "chosen-by-user")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("YAML 空值压过了环境变量: %v", err)
	}
	if cfg.JWT.Secret != "generated-from-env" {
		t.Errorf("jwt.secret = %q, want 环境变量的值", cfg.JWT.Secret)
	}
	if cfg.Admin.JWTSecret != "generated-admin-from-env" {
		t.Errorf("admin.jwt_secret = %q, want 环境变量的值", cfg.Admin.JWTSecret)
	}
}

// TestLoadRejectsInvalidPortsAndListenAddr 端口范围与后台监听地址的校验
// 由启动路径执行（schema.go validationRules）。
//
// 后端跑的是直接二进制部署，配置不经过任何界面——校验必须落在启动路径上，
// 否则 `config.yaml` 里手写 listen_addr: 0.0.0.0 就足以把后台端口摊到公网。
func TestLoadRejectsInvalidPortsAndListenAddr(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")

	cases := []struct{ name, body string }{
		{"公开端口为 0", "server:\n  port: 0\n  mode: test\njwt:\n  secret: s\nadmin:\n  jwt_secret: a\n"},
		{"运行模式非法", "server:\n  port: 9090\n  mode: prod\njwt:\n  secret: s\nadmin:\n  jwt_secret: a\n"},
		{"用户 token 有效期为零", "server:\n  port: 9090\n  mode: test\njwt:\n  secret: s\n  expire_min: 0\nadmin:\n  jwt_secret: a\n"},
		{"后台 token 有效期越界", "server:\n  port: 9090\n  mode: test\njwt:\n  secret: s\nadmin:\n  jwt_secret: a\n  expire_min: 700000\n"},
		{"后台端口越界", "server:\n  port: 9090\n  mode: test\njwt:\n  secret: s\nadmin:\n  jwt_secret: a\n  listen_port: 70000\n"},
		{"监听通配", "server:\n  port: 9090\n  mode: test\njwt:\n  secret: s\nadmin:\n  jwt_secret: a\n  listen_addr: 0.0.0.0\n"},
		{"监听未指定 IPv6（短写法）", "server:\n  port: 9090\n  mode: test\njwt:\n  secret: s\nadmin:\n  jwt_secret: a\n  listen_addr: ::0\n"},
		{"监听未指定 IPv6（全零写法）", "server:\n  port: 9090\n  mode: test\njwt:\n  secret: s\nadmin:\n  jwt_secret: a\n  listen_addr: 0:0:0:0:0:0:0:0\n"},
		{"监听未指定 IPv4 映射写法", "server:\n  port: 9090\n  mode: test\njwt:\n  secret: s\nadmin:\n  jwt_secret: a\n  listen_addr: '::ffff:0.0.0.0'\n"},
		{"监听主机名", "server:\n  port: 9090\n  mode: test\njwt:\n  secret: s\nadmin:\n  jwt_secret: a\n  listen_addr: my-host.local\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Load(writeTempYaml(t, c.body)); err == nil {
				t.Fatal("非法配置被接受了")
			}
		})
	}
}

// TestLoadNormalizesModeWhitespace 规则表按去空白后的值判断 mode，写回也必须是去空白值。
//
// 否则 `mode: " debug "` 能通过校验，却在 main 里交给 gin.SetMode 时 panic——
// 配置校验存在的意义就是不让配置问题变成进程崩溃。
func TestLoadNormalizesModeWhitespace(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")

	path := writeTempYaml(t, `server:
  port: 9090
  mode: " debug "
jwt:
  secret: s
admin:
  jwt_secret: a
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Server.Mode != "debug" {
		t.Fatalf("server.mode = %q, want debug", cfg.Server.Mode)
	}
}

// TestLoadNormalizesTrimmedRuleFields 规则表去空白判断的字段，落到运行时也必须是去空白值。
//
// 校验用 TrimSpace 比较、程序却使用带空白的原值时，「通过校验」与「能用」是两件事：
// admin.username 是这里最贵的一例——后台登录拿请求里的用户名与配置值做常量时间比较，
// `username: " admin "` 会让真正的管理员永远登不进去，而 config.Load 一路绿灯、
// 启动时不出一句错。密钥与上传目录同理：签出去的 HMAC key 不该是「配置里那串加两格空白」。
func TestLoadNormalizesTrimmedRuleFields(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET", "ADMIN_PASSWORD")

	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
jwt:
  secret: "  spaced-secret  "
admin:
  username: " admin "
  jwt_secret: "  spaced-admin-secret  "
upload:
  dir: " uploads "
smtp:
  host: "   "
  port: 465
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if got := cfg.Admin.Username; got != "admin" {
		t.Errorf("admin.username = %q, want admin（否则后台按 'admin' 登录必然失败）", got)
	}
	if got := cfg.JWT.Secret; got != "spaced-secret" {
		t.Errorf("jwt.secret = %q, want spaced-secret", got)
	}
	if got := cfg.Admin.JWTSecret; got != "spaced-admin-secret" {
		t.Errorf("admin.jwt_secret = %q, want spaced-admin-secret", got)
	}
	if got := cfg.Upload.Dir; got != "uploads" {
		t.Errorf("upload.dir = %q, want uploads", got)
	}
	// 只有空白的 smtp.host 收敛成空串 = 「未配置 SMTP」：它同时是 smtp.port 规则的
	// When 依赖，留着空白会让这条依赖判断与实际发信行为各认一份事实。
	if got := cfg.SMTP.Host; got != "" {
		t.Errorf("smtp.host = %q, want 空串（空白等同于未配置）", got)
	}
}

// TestLoadNormalizesEmptyAdminListenAddr 显式空串必须收敛为回环地址。
//
// viper 的显式空值优先于 SetDefault：若把 "" 原样交给 net.JoinHostPort，
// 得到的是 ":9091"，net.Listen 会绑定所有网卡——ADR-0010 的网络隔离失效。
// 空串在文档中的含义一直是「用默认回环地址」，这里钉住它真的落到绑定值上。
func TestLoadNormalizesEmptyAdminListenAddr(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")

	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
jwt:
  secret: s
admin:
  jwt_secret: a
  listen_addr: ""
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Admin.ListenAddr != "127.0.0.1" {
		t.Fatalf("admin.listen_addr = %q, want 127.0.0.1", cfg.Admin.ListenAddr)
	}
}

// TestLoadRejectsBuiltInSecretsInRelease 生产模式不得用仓库里可读的默认密钥。
//
// 默认值是为「克隆下来就能跑」准备的，但同一份字符串在公开仓库里；带着它上线
// 与空密钥是同一个洞。部署者通常把密钥放 .env，此时 YAML 里已无该键、默认值会
// 重新生效——若 .env 意外丢失，这条拒绝就是最后的兜底。
func TestLoadRejectsBuiltInSecretsInRelease(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")

	path := writeTempYaml(t, `server:
  port: 9090
  mode: release
jwt:
  secret: ebook-secret-key
admin:
  jwt_secret: ebook-admin-secret-key
`)

	if _, err := Load(path); err == nil {
		t.Fatal("release 模式带着内置默认密钥启动成功")
	}
}

// TestLoadAllowsExplicitEmptyAdminPassword 空管理员口令是合法的显式状态。
//
// 配置文件可以保留 password: "" 表示「尚未设置」；它既不能被 SetDefault 换回
// 内置口令，也不能被当成缺键拒绝启动。空口令不可被利用：后台登录要求非空凭据。
func TestLoadAllowsExplicitEmptyAdminPassword(t *testing.T) {
	unsetEnv(t, "ADMIN_PASSWORD")
	t.Setenv("JWT_SECRET", "jwt-from-env")
	t.Setenv("ADMIN_JWT_SECRET", "admin-jwt-from-env")

	path := writeTempYaml(t, `server:
  port: 9090
  mode: release
admin:
  password: ""
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("显式空管理员口令不应阻止启动: %v", err)
	}
	if cfg.Admin.Password != "" {
		t.Fatalf("admin.password = %q, want 空串（显式空值必须保留）", cfg.Admin.Password)
	}
	if cfg.Admin.ListenAddr != "127.0.0.1" {
		t.Fatalf("未写 listen_addr 时应使用默认回环地址, got %q", cfg.Admin.ListenAddr)
	}
}

// TestLoadAllowsBuiltInSecretsOutsideRelease 开发模式的零配置启动不受影响。
//
// 这条拒绝针对的是「上线」而不是「跑起来看看」：debug/test 下继续用默认值，
// 否则 AGENTS.md 承诺的本地零配置体验就换成了每条 interface 的前置负担。
func TestLoadAllowsBuiltInSecretsOutsideRelease(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")

	for _, mode := range []string{"debug", "test"} {
		t.Run(mode, func(t *testing.T) {
			path := writeTempYaml(t, "server:\n  port: 9090\n  mode: "+mode+"\n")

			if _, err := Load(path); err != nil {
				t.Fatalf("%s 模式的零配置启动被拒绝: %v", mode, err)
			}
		})
	}
}

// TestLoadRejectsEmptyUploadDir 空的 upload.dir 会把工作目录变成公开文件服务。
//
// /uploads 静态挂载的根就是它；空串在 net/http 里解析为进程当前目录，
// 服务端的工作目录下正躺着 config.yaml、.env 与 ebook.db。
func TestLoadRejectsEmptyUploadDir(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")

	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
upload:
  dir: ""
`)

	if _, err := Load(path); err == nil {
		t.Fatal("upload.dir 为空时被接受：/uploads 会暴露整个工作目录")
	}
}

// TestLoadRejectsShippedPlaceholderSecretsInRelease 生产模式不得用仓库里公开的占位凭据。
//
// .env.example 与 config.yaml 里的样板值（your-secret-key-change-this /
// change-me-admin / your-admin-secret-change-this）同样在公开仓库里可读。把
// .env.example 原样复制成 .env 就上线，等于用一把全网都知道的密钥签名、用一个
// 全网都知道的口令登录后台——和用内置默认值是同一类洞，只是来源不同。
func TestLoadRejectsShippedPlaceholderSecretsInRelease(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET", "ADMIN_PASSWORD")

	for _, tc := range []struct {
		name, body, want string
	}{
		{"公开密钥是占位值", "jwt:\n  secret: your-secret-key-change-this\nadmin:\n  jwt_secret: table-secret\n", "jwt.secret"},
		{"后台密钥是占位值", "jwt:\n  secret: table-secret\nadmin:\n  jwt_secret: your-admin-secret-change-this\n", "admin.jwt_secret"},
		// 同一段落里写两个键：duplicate 的 `admin:` 会让 yaml 解码先失败，
		// validate 根本没跑到，用例就成了「无论占位值与拒绝逻辑如何都绿」。
		{"管理员口令是占位值", "jwt:\n  secret: table-secret\nadmin:\n  jwt_secret: table-admin-secret\n  password: change-me-admin\n", "admin.password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempYaml(t, "server:\n  port: 9090\n  mode: release\n"+tc.body)

			_, err := Load(path)
			if err == nil {
				t.Fatal("release 模式带着仓库公开的占位凭据启动成功")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want 提到 %s（占位值校验没有生效）", err, tc.want)
			}
		})
	}
}

// TestLoadAllowsShippedPlaceholderSecretsOutsideRelease 开发模式不拦占位值。
//
// 这条拒绝针对的是「上线」：debug/test 下继续用仓库里的样板值，否则本地零配置
// 启动（不改任何文件直接 make run）就被挡住了。
func TestLoadAllowsShippedPlaceholderSecretsOutsideRelease(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET", "ADMIN_PASSWORD")

	for _, mode := range []string{"debug", "test"} {
		t.Run(mode, func(t *testing.T) {
			path := writeTempYaml(t, "server:\n  port: 9090\n  mode: "+mode+
				"\njwt:\n  secret: your-secret-key-change-this\nadmin:\n  jwt_secret: your-admin-secret-change-this\n")

			if _, err := Load(path); err != nil {
				t.Fatalf("%s 模式被占位值挡住启动: %v", mode, err)
			}
		})
	}
}

// TestLoadDatabasePathEnvOverride DATABASE_PATH 必须真的覆盖配置文件。
//
// viper 的 Unmarshal 只遍历「已知键」；没登记的键即使开了 AutomaticEnv 也整段
// 被跳过——配置文件里没有 database 段时，环境变量里的 DATABASE_PATH 读不到，
// 数据库路径静默退回缺省文件名。键登记在 schema.go 的 fieldDefaults 里。
func TestLoadDatabasePathEnvOverride(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")
	t.Setenv("DATABASE_PATH", "from-env.db")

	// 刻意不写 database 段：这一段缺失正是覆盖失效的触发条件。
	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
jwt:
  secret: table-secret
admin:
  jwt_secret: table-admin-secret
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Database.Path != "from-env.db" {
		t.Fatalf("database.path = %q, want %q（环境变量覆盖被静默丢弃）",
			cfg.Database.Path, "from-env.db")
	}
}

// TestLoadSmtpEnvOverrides 配置文件里没有 smtp 段时，环境变量仍要生效。
//
// .env.example 承诺 SMTP_HOST / SMTP_PORT / SMTP_USERNAME / SMTP_FROM 可以覆盖
// 配置。它们不是秘密，但必须显式登记成已知键：
// viper 的 Unmarshal 只遍历已知键，靠 AutomaticEnv 兜底在配置文件缺段时读不到。
func TestLoadSmtpEnvOverrides(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")
	t.Setenv("SMTP_HOST", "smtp.env.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "env@example.com")
	t.Setenv("SMTP_FROM", "from-env@example.com")

	// 刻意不写 smtp 段。
	path := writeTempYaml(t, `server:
  port: 9090
  mode: test
jwt:
  secret: table-secret
admin:
  jwt_secret: table-admin-secret
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.SMTP.Host != "smtp.env.example.com" {
		t.Errorf("smtp.host = %q, want smtp.env.example.com", cfg.SMTP.Host)
	}
	if cfg.SMTP.Port != 587 {
		t.Errorf("smtp.port = %d, want 587", cfg.SMTP.Port)
	}
	if cfg.SMTP.Username != "env@example.com" {
		t.Errorf("smtp.username = %q, want env@example.com", cfg.SMTP.Username)
	}
	if cfg.SMTP.From != "from-env@example.com" {
		t.Errorf("smtp.from = %q, want from-env@example.com", cfg.SMTP.From)
	}
}

// TestLoadRejectsMalformedDotEnv 语法错误的 .env 必须当场失败，而不是静默跳过。
//
// 吞掉它有两种坏结果：debug 模式带着内置密钥继续跑；release 模式把「.env 没读进来、
// 密钥仍是默认值」报成一句与真正原因无关的校验失败。只有「文件不存在」可以忽略。
func TestLoadRejectsMalformedDotEnv(t *testing.T) {
	unsetEnv(t, "JWT_SECRET", "ADMIN_JWT_SECRET")

	// godotenv.Load 读工作目录下的 .env；其余用例都用绝对路径的临时配置，不受影响。
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("这行不是 KEY=VALUE\n"), 0o600); err != nil {
		t.Fatalf("写入 .env 失败: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("恢复工作目录失败: %v", err)
		}
	}()

	path := writeTempYaml(t, "server:\n  port: 9090\n  mode: test\n")

	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), ".env") {
		t.Fatalf("err = %v, want 提到 .env 的解析失败", err)
	}
}
