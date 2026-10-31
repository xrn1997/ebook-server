# 配置契约由 Go 单一来源生成

Status: accepted
Superseded in part by: 0024（§2 生成物与桌面执行器；Go 单一来源与校验保留）

> **部分作废（ADR-0024）**：本 ADR 的 §2 建立在「配置要同时供桌面端与 Go 后端使用」
> 之上。后台收敛为内嵌 web 后，配置只由部署者通过 `config.yaml`/`.env` 维护，不再有
> 第二份跨语言契约需要生成与比对，生成器与逐字节比对用例一并撤销。
> §1 的 Go 单一来源、§3 的内置凭据边界与 §4 的启动校验仍然有效。

配置的字段清单曾经有三份：`config.Config` 结构体、`config.setDefaults` 的逐行注册、
桌面端 `desktop/src/main/config.ts`（已删）的 `SCHEMA`。加一个键要记得改三处，漏一处的后果不对称：

* 漏在 Go：`Unmarshal` 读不到（viper 只认它知道的键），配置静默失效。
* 漏在桌面：界面保存时把它当未知键丢弃，用户改了也不生效。
* 两端的校验规则也各写一份：桌面拦端口与监听地址，Go 只拦空密钥。于是**直接跑二进制**
  时，`admin.listen_addr: 0.0.0.0` 能通过校验，把后台端口摊到公网（ADR-0010 失效）。

## 决策

### 1. `backend/config` 是唯一来源（`Config` 标签 + `schema.go` 规则）

* 字段清单来自 `Config` 的 `mapstructure` 标签；`schema_test.go` 的
  `TestEveryConfigKeyIsRegisteredForViper` 反向遍历结构体，保证每个叶子键都登记进
  `fieldDefaults`——漏登记的键即使写在 YAML 里也不会被 `SetDefault`/`BindEnv` 覆盖，
  环境变量对该键直接失效。
* `fieldDefaults` 是默认值与环境变量绑定的唯一注册表；`setDefaults` 只负责按表执行
  `SetDefault` 与 `BindEnv`。
* `validationRules()` 是业务校验规则表（端口范围、非空密钥、后台监听地址禁通配/
  未指定地址/主机名），启动校验按同一份文案拒绝。后台监听地址的「未指定地址」
  按解析结果判定（`0.0.0.0`、`::`、`::0`、全零 IPv6 写法），不是有限字符串名单。

### 2. 生成物入库，逐字节比对防漂移（已随 ADR-0024 撤销）

桌面端曾有一份由 Go 渲染的 TypeScript 配置契约：生成物入库，并用逐字节比对钉住
「改了 Go 侧却忘记重新生成」。后台收敛为内嵌 web 后，配置不再需要第二份跨语言契约，
生成器、生成物与比对用例一并撤销——配置契约只剩 Go 这一份。

### 3. 内置凭据只在本地开发可用

三份代码内置凭据（`jwt.secret`、`admin.jwt_secret`、`admin.password`）公开在仓库里。
它们仍登记在 `fieldDefaults` 中，让 clone 下来的仓库在 debug/test 模式零配置启动；
`release` 模式下 `config.validate` 会拒绝仍等于内置值或仓库占位值的配置——缺键回落到
内置默认值同样会被拦下。`fieldDefaults` 用 `BuiltInSecret` 标出这三项，校验不另抄
字段清单；占位值清单（`.env.example` 的样板串）同样在 `schema.go`。

### 4. 后端也执行完整规则表

`config.validate` = `validateRules` + release 模式的 `builtInSecrets` 检查。
直接部署的二进制不再绕过端口范围与后台监听地址的校验——这是唯一的执行点，
配置写错时进程当场拒绝启动，而不是带着敞开的后台端口继续跑。

显式空的 `admin.listen_addr: ""` 会在 `Load` 时归一为 `127.0.0.1`：viper 的显式
空值优先于 `SetDefault`，而 `net.JoinHostPort("", port)` 的绑定语义是**所有网卡**
（ADR-0010 失效）。空串继续表示「用默认回环地址」，但必须在进入 `net.Listen`
之前变成实际值；真正的通配/未指定地址仍然拒绝，不静默改写。

## 后果

* 新增配置项：Go 结构体字段（`mapstructure` 标签）→ `fieldDefaults` 默认值/环境变量 →
  `validationRules` 与 `config` 测试。
* 配置契约由 Go 单一来源校验；原桌面表单与 TypeScript 生成物已随 ADR-0024 移除。
* ADR-0012 的桌面边界已撤销：当前部署只通过 `config.yaml`/`.env` 维护配置，不经过浏览器后台。

测试：`config/schema_test.go`（键覆盖、规则拒绝非法值）、
`config/config_test.go`（非法端口/监听地址启动失败、env 覆盖契约）。

锚点：`backend/config/schema.go`；`backend/config/schema_test.go`；ADR-0024。
