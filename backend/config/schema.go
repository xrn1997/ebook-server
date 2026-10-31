package config

import (
	"net"
	"net/url"
	"reflect"
	"sort"
	"strings"
)

// 本文件与 config.go 的 Config 标签共同构成配置契约的单一来源：字段清单来自
// Config 的 mapstructure 标签，默认值、环境变量覆盖与校验规则定义在这里。
//
// 后端（viper 装配与 validate）从这里派生；执行器刻意保持薄，定义只有这一份。

// fieldDefault 默认值与环境变量绑定的唯一注册表（setDefaults 按表执行）。
type fieldDefault struct {
	Section string
	Key     string
	// Value 是默认值；nil 表示没有内置默认值（键仍会被登记，见 fieldDefaults 的说明）。
	Value any
	// Env 非空表示该键可由这个（大写）环境变量覆盖。
	Env string
	// BuiltInSecret 标记「公开在仓库里的内置凭据」：可以留在源码里供本地零配置
	// 启动，但 release 模式下必须拒绝。
	BuiltInSecret bool
}

// 内置默认密钥/口令：只为「克隆下来就能跑」准备，同一份字符串在公开仓库里可读。
const (
	defaultJWTSecret     = "ebook-secret-key"
	defaultAdminJWT      = "ebook-admin-secret-key"
	defaultAdminPassword = "admin123-dev"
	// defaultAdminListenAddr 后台监听地址的默认值；下面的默认值注册表与
	// normalize 的显式空值回落共用同一常量。
	defaultAdminListenAddr = "127.0.0.1"
)

// fieldDefaults 是本文件唯一的默认值清单；这里只提供按 section.key 的查找。
//
// 每个条目都把键登记进 viper 的已知键集合，这一点比默认值本身更重要：viper 的
// Unmarshal 只遍历已知键（默认值 / 配置文件 / BindEnv），没登记的键即使开了
// AutomaticEnv 也会被整个跳过——配置文件里没有那一段时，环境变量里的同名值
// 读不到。
//
// database.path 在此登记但刻意**不**给默认值：缺省文件名由 pkg/database.DefaultPath
// 决定，这里再写一份就成了两个真相来源。登记它是为了让 DATABASE_PATH 这类覆盖
// 真的能生效，并让配置加载知道这个键存在。
var fieldDefaults = []fieldDefault{
	{Section: "server", Key: "port", Value: 9090},
	{Section: "server", Key: "mode", Value: "debug"},
	// 公开资源（头像）URL 的来源。留空 = 按请求 Host 拼接，所以它没有内置默认值，
	// 但必须登记：不登记的话 SERVER_PUBLIC_BASE_URL 覆盖压根不生效。
	{Section: "server", Key: "public_base_url", Value: "", Env: "SERVER_PUBLIC_BASE_URL"},
	{Section: "jwt", Key: "secret", Value: defaultJWTSecret, Env: "JWT_SECRET", BuiltInSecret: true},
	{Section: "jwt", Key: "expire_min", Value: 120},
	{Section: "database", Key: "path", Env: "DATABASE_PATH"},
	{Section: "admin", Key: "username", Value: "admin"},
	{Section: "admin", Key: "password", Value: defaultAdminPassword, Env: "ADMIN_PASSWORD", BuiltInSecret: true},
	{Section: "admin", Key: "jwt_secret", Value: defaultAdminJWT, Env: "ADMIN_JWT_SECRET", BuiltInSecret: true},
	{Section: "admin", Key: "expire_min", Value: 60},
	{Section: "admin", Key: "listen_addr", Value: defaultAdminListenAddr},
	{Section: "admin", Key: "listen_port", Value: 9091},
	// SMTP 的主机/端口/账号/发件人可由环境变量覆盖（.env.example 的承诺），但不是
	// 内置凭据。
	{Section: "smtp", Key: "host", Env: "SMTP_HOST"},
	{Section: "smtp", Key: "port", Env: "SMTP_PORT"},
	{Section: "smtp", Key: "username", Env: "SMTP_USERNAME"},
	{Section: "smtp", Key: "password", Value: "", Env: "SMTP_PASSWORD"},
	{Section: "smtp", Key: "from", Env: "SMTP_FROM"},
	{Section: "smtp", Key: "insecure", Value: false},
	{Section: "upload", Key: "dir", Value: "uploads"},
	{Section: "api_docs", Key: "enabled", Value: false},
}

// normalize 把「显式空值」收敛成安全默认，再交给 validate。
//
// viper 的显式空值优先于 SetDefault：`admin.listen_addr: ""` 会让
// net.JoinHostPort("", port) 得到 ":port"，net.Listen 在该形态下绑定**所有网卡**——
// ADR-0010 的后台网络隔离当场失效。空串在文档中的含义是「用默认回环地址」，
// 这里把这句话落到实际绑定值上；通配/未指定地址仍由 validate 拒绝，不静默改写。
func (c *Config) normalize() {
	// mode 会原样交给 gin.SetMode；规则表按去空白后的值匹配，这里必须把同一个
	// 值真正写回，否则 `mode: " debug "` 会通过校验却在 SetMode 时 panic。
	c.Server.Mode = strings.TrimSpace(c.Server.Mode)

	// 同一个理由覆盖规则表里其余按 TrimSpace 匹配的字符串字段：**校验看到的值必须
	// 就是运行时使用的值**，否则规则通过的是一个值、程序用的是另一个带空白的兄弟。
	//
	// 后果最实在的是 admin.username：登录把请求里的用户名与配置值做常量时间比较
	// （internal/admin/auth.go），`username: " admin "` 会通过 config.Load 却永远
	// 匹配不上「admin」——后台被永久锁死，而且启动时一声不响。
	// jwt.secret / admin.jwt_secret 不收敛时，密钥字面上带着空白参与 HMAC，
	// 与「配置里写的那串」不是同一个值；upload.dir 则会把带空白的目录名当路径用。
	c.Admin.Username = strings.TrimSpace(c.Admin.Username)
	c.JWT.Secret = strings.TrimSpace(c.JWT.Secret)
	c.Admin.JWTSecret = strings.TrimSpace(c.Admin.JWTSecret)
	c.Upload.Dir = strings.TrimSpace(c.Upload.Dir)
	// smtp.host 是 smtp.port 那条 range 规则的 When 依赖：只写空白的 host 若原样留着，
	// 「没配 SMTP」与「配了个空白」在规则眼里是同一件事，端口检查会被整段跳过。
	c.SMTP.Host = strings.TrimSpace(c.SMTP.Host)

	c.Admin.ListenAddr = strings.TrimSpace(c.Admin.ListenAddr)
	if c.Admin.ListenAddr == "" {
		c.Admin.ListenAddr = defaultAdminListenAddr
	}
	// public_base_url 进 URL 拼接，带空白就是另一个 origin；这里收敛成校验看到的值。
	c.Server.PublicBaseURL = strings.TrimSpace(c.Server.PublicBaseURL)
}

// shippedPlaceholderSecrets 是仓库里公开的占位凭据：.env.example 与 config.yaml
// 的样板值。
//
// 它们不是代码内置默认值，但同样不能带到 release：把 .env.example 原样复制成
// .env 就上线，等于用一把全网都读得到的密钥签名、用一个全网都知道的口令登录后台。
// release 模式必须把它们和内置默认值一样拒掉。
//
// 键是 section.key，值是该键在仓库里出现过的占位串。
var shippedPlaceholderSecrets = map[string][]string{
	"jwt.secret":       {"your-secret-key-change-this"},
	"admin.password":   {"change-me-admin"},
	"admin.jwt_secret": {"your-admin-secret-change-this", "your-admin-jwt-secret-change-this"},
}

// placeholderSecrets 返回当前配置里仍是仓库公开占位值的字段名（按字典序）。
func placeholderSecrets(values map[string]any) []string {
	var weak []string
	for key, placeholders := range shippedPlaceholderSecrets {
		value, _ := values[key].(string)
		for _, placeholder := range placeholders {
			if value == placeholder {
				weak = append(weak, key)
				break
			}
		}
	}
	sort.Strings(weak)
	return weak
}

// wildcardListenAddrs 绑定语义为「所有网卡」的监听地址。
var wildcardListenAddrs = []string{"0.0.0.0", "::", "[::]", "*"}

// ruleKind 校验规则的类型。
type ruleKind string

const (
	// ruleNonEmpty 字段必须是非空白字符串。
	ruleNonEmpty ruleKind = "nonempty"
	// ruleRange 数值字段必须落在 [Min, Max]；When 非空时依赖字段为空则跳过。
	ruleRange ruleKind = "range"
	// ruleForbid 字段（去空白后）不得等于 Values 中任一值。
	ruleForbid ruleKind = "forbid"
	// ruleOneOf 字段（去空白后）必须等于 Values 中之一。
	ruleOneOf ruleKind = "oneof"
	// ruleUnspecified 字段不得解析为「未指定地址」（0.0.0.0、:: 及其等价 IPv6 写法）。
	// 这类地址的绑定语义同样是所有网卡，精确字符串名单挡不住 ::0、0:0:0:0:0:0:0:0。
	ruleUnspecified ruleKind = "unspecified"
	// ruleHost 字段必须是空串、localhost、通配地址或 IP 字面量（不能是主机名）。
	ruleHost ruleKind = "host"
	// ruleOrigin 字段为空，或是一个**纯 origin**：http/https、有主机、不带路径/查询/用户信息。
	// 用在 server.public_base_url 上——那个值只用来拼 <origin>/uploads/avatar/<file>。
	ruleOrigin ruleKind = "origin"
)

// validationRule 一条业务校验规则。
type validationRule struct {
	Field   string
	Kind    ruleKind
	Min     int
	Max     int
	When    string
	Values  []string
	Message string
}

// validationRules 配置的业务校验规则表。
//
// 消息文案直接面向部署者（启动失败时原样显示），所以写「怎么做」而不是「哪里错了」。
func validationRules() []validationRule {
	return []validationRule{
		{Field: "server.port", Kind: ruleRange, Min: 1, Max: 65535, Message: "server.port 必须在 1-65535 之间"},
		{Field: "server.mode", Kind: ruleOneOf, Values: []string{"debug", "release", "test"}, Message: "server.mode 必须是 debug、release 或 test"},
		{Field: "admin.listen_port", Kind: ruleRange, Min: 1, Max: 65535, Message: "admin.listen_port 必须在 1-65535 之间"},
		{Field: "smtp.port", Kind: ruleRange, Min: 1, Max: 65535, When: "smtp.host", Message: "smtp.port 必须在 1-65535 之间"},
		{Field: "jwt.expire_min", Kind: ruleRange, Min: 1, Max: 525600, Message: "jwt.expire_min 必须在 1-525600 之间"},
		{Field: "admin.expire_min", Kind: ruleRange, Min: 1, Max: 525600, Message: "admin.expire_min 必须在 1-525600 之间"},
		{Field: "jwt.secret", Kind: ruleNonEmpty, Message: "jwt.secret 不能为空"},
		{Field: "admin.jwt_secret", Kind: ruleNonEmpty, Message: "admin.jwt_secret 不能为空"},
		{Field: "admin.username", Kind: ruleNonEmpty, Message: "admin.username 不能为空"},
		{Field: "upload.dir", Kind: ruleNonEmpty, Message: "upload.dir 不能为空"},
		{Field: "admin.listen_addr", Kind: ruleForbid, Values: wildcardListenAddrs, Message: "admin.listen_addr 不允许使用通配地址（ADR-0010 网络隔离），请填 127.0.0.1 或具体内网 IP"},
		{Field: "admin.listen_addr", Kind: ruleUnspecified, Message: "admin.listen_addr 不允许使用通配地址（ADR-0010 网络隔离），请填 127.0.0.1 或具体内网 IP"},
		{Field: "admin.listen_addr", Kind: ruleHost, Values: wildcardListenAddrs, Message: "admin.listen_addr 必须是 IP 地址或 localhost，不能填主机名"},
		{Field: "server.public_base_url", Kind: ruleOrigin, Message: "server.public_base_url 要么留空（按请求 Host 拼接），要么填纯 origin，例如 https://book.example.com（不带路径与查询）"},
	}
}

// validateRules 按规则表校验配置，返回问题列表（空 = 通过）。
func validateRules(c *Config) []string {
	values := fieldValues(c)
	var problems []string

	for _, rule := range validationRules() {
		value, ok := values[rule.Field]
		if !ok {
			continue
		}
		switch rule.Kind {
		case ruleNonEmpty:
			if s, _ := value.(string); strings.TrimSpace(s) == "" {
				problems = append(problems, rule.Message)
			}
		case ruleRange:
			if rule.When != "" {
				dep, _ := values[rule.When].(string)
				if strings.TrimSpace(dep) == "" {
					continue
				}
			}
			if n, _ := value.(int); n < rule.Min || n > rule.Max {
				problems = append(problems, rule.Message)
			}
		case ruleForbid:
			if s, _ := value.(string); containsValue(rule.Values, strings.TrimSpace(s)) {
				problems = append(problems, rule.Message)
			}
		case ruleOneOf:
			if s, _ := value.(string); !containsValue(rule.Values, strings.TrimSpace(s)) {
				problems = append(problems, rule.Message)
			}
		case ruleUnspecified:
			s, _ := value.(string)
			if ip := net.ParseIP(strings.TrimSpace(s)); ip != nil && ip.IsUnspecified() {
				problems = append(problems, rule.Message)
			}
		case ruleHost:
			host, _ := value.(string)
			host = strings.TrimSpace(host)
			if host == "" || host == "localhost" || containsValue(rule.Values, host) {
				continue
			}
			if net.ParseIP(host) == nil {
				problems = append(problems, rule.Message)
			}
		case ruleOrigin:
			// 空值合法：留空即「按请求 Host 拼接」，自托管零配置仍能跑（ADR-0011）。
			if s, _ := value.(string); !isOrigin(s) {
				problems = append(problems, rule.Message)
			}
		}
	}

	// 精确名单与「未指定地址」两条规则对 0.0.0.0 / :: 会给出同一句文案，去重后再返回：
	// 调用方拿到的是一组问题，不是同一句话的重复次数。
	seen := make(map[string]struct{}, len(problems))
	unique := problems[:0]
	for _, problem := range problems {
		if _, ok := seen[problem]; ok {
			continue
		}
		seen[problem] = struct{}{}
		unique = append(unique, problem)
	}
	return unique
}

// fieldValues 用反射把 Config 摊平成「section.key → 值」，顺序无关。
func fieldValues(c *Config) map[string]any {
	values := make(map[string]any, 16)
	v := reflect.ValueOf(c).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		section := t.Field(i)
		sectionName, ok := section.Tag.Lookup("mapstructure")
		if !ok || section.Type.Kind() != reflect.Struct {
			continue
		}
		sectionValue := v.Field(i)
		for j := 0; j < section.Type.NumField(); j++ {
			leaf := section.Type.Field(j)
			key, ok := leaf.Tag.Lookup("mapstructure")
			if !ok {
				continue
			}
			values[sectionName+"."+key] = sectionValue.Field(j).Interface()
		}
	}
	return values
}

// containsValue 名单命中判断。
func containsValue(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// isOrigin 判断「空」或「纯 origin」（scheme 为 http/https、有主机、无路径/查询/用户信息）。
//
// 子路径被排除是有意的：server.public_base_url 只用来拼 <origin>/uploads/avatar/<file>，
// 填了子目录的人以为配的是前缀，实际会得到两段路径拼在一起的错误 URL——
// 与其安静地产出坏链接，不如启动时就拒掉。唯一的例外是单个尾斜杠：`https://host/`
// 与 `https://host` 是同一个 origin（url.Parse 把它解析成 Path="/"），而前者是部署者
// 最常写的形态；upload.New 也按「带不带尾斜杠给出同一个 URL」处理它。
func isOrigin(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return true
}
