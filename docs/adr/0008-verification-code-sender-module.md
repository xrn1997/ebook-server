# 发码编排收拢进验证码发送纵深模块

将「下发验证码」（流程化命名空间键 → 双窗口限流 → 存码 → 发邮件）从三个流程的重复编排，
收拢进 `service` 包内的 `VerificationCodeSender` 纵深模块：限流器与流程键符号表随模块归位，
sender 对账号无感（枚举隐藏仍留在找回流程，ADR-0006）。

## 现状

「给某邮箱在某流程下发一个验证码」在三个地方被复制了一遍：

* `service/auth.go` `SendCode`（注册，不限账号存在性）
* `service/auth.go` `SendForgotCode`（找回密码，先查账号隐藏枚举）
* `service/account.go` `SendDeletionCode`（注销发码）

三处各自挂着**一对限流器**（`sendCodeMinute`/`sendCodeHour`，每分钟 1 次、每小时 5 次）、
各自调 `allowSendCode` + `codes.Save(flow+email)` + `mailer.SendCode`。限流器存在状态
**散落在每个 service 实例上**，而不是「下发验证码」这一域概念的一个属性。
流程前缀裸字符串（`"reg:"`/`"forgot:"`/`"del:"`）同时充当存储命名空间与限流配额键，
这份隐式耦合没有归属。

ADR-0007 已定调：`code.Store`、`ratelimit.Limiter` 以具体类型注入、不立接口
（第二适配器 Redis 出现前不建 seam）。

## 动机

* **真实重复、收益集中的浅**：三份编排几乎相同，删除测试通过——删掉它让复杂度收拢而非挪位。

* **限流属权错置**：限流是「发码」的防守，状态却被 service 实例绑架；想要同一套限流语义、
  又要流程独立配额，就得在每次新增流程时再复制一份。

* **键约定无主人**：存储键 = 限流键 = 流程前缀，散在三处常量，一旦改编码键而不改限流键，
  两个流程的配额会被意外合并。

## 决策

1. **新增 `service/verification.go`**：`VerificationCodeSender` 纵深模块，`main.go` 建唯一实例
   注入认证/账号两模块。对外只露 `Send(flow, email) error`。

2. **全权归属限流 + 键符号表 + 存码 + 发信**：sender 内在拥有一对限流器（按 flow+email 隔离配额）、
   流程前缀符号表、`codes.Save`、`mailer.SendCode`。对「注册→找回→注销」的限流语义一处可改。

3. **`Flow` 类型化枚举（`register`/`forgot`/`deletion`）**：前缀映射（`":"` 串）是 sender 的私有符号表，
   对外不暴露；存储命名空间与限流配额同源、新增流程只加一枚举值 + 一处前缀映射。

4. **sender 对账号无感**：不持有 `UserStore`。找回密码的「账号不存在也成功」是**防枚举策略**
   （ADR-0006），留在 `SendForgotCode` 调用点，不进 sender。

5. **保持纯内部结构变更**：任何 API 请求/响应与改造前完全一致（ADR-0007 §验收 同款保证）。

## 被拒方案

* **sender 只打包存储+邮件、限流留在调用方**：只是把复杂度移走而非收拢，浅一半，删除测试不过。

* **sender 感知账号（`SendOnlyIfExists` 变体）**：让「发码」依赖「账号」，多一条依赖轴，稀释纵深；
  枚举隐藏是流程策略而非发码职责。

* **sender 入住 `pkg/verification` / `pkg/code`**：把业务流程编排当基础库对待，需为 `Flow` 枚举与
  限流参数在包间立契约，撑大接口；它与 ADR-0007「service 内聚编排」相悖。

## 下游影响

* `service/verification.go`：新增 `VerificationCodeSender`、`Flow` 类型与前缀符号表。
* `service/auth.go`：`SendCode`/`SendForgotCode` 改调 sender；删减内部限流器字段、`allowSendCode` 相关私有编排。
* `service/account.go`：`SendDeletionCode` 改调 sender。
* `service/ports.go`：`newSendCodeLimiters`/`allowSendCode` 迁移进 sender。
* `main.go`：装配唯一 sender 实例并注入。
* `CONTEXT.md`：新增「验证码」术语，记录流程隔离语义。
* 测试：新增 sender 聚焦测试；认证/账号模块测试改经 sender 路径，行为断言不变。

## 验收

1. `go build` / `go vet` / `gofmt -l` / `go test -count=1 ./...` 全绿。
2. 除 `main.go` 外，生产代码不再出现 `newSendCodeLimiters` 的逐 service 复制；限流器状态只属 sender。
3. 注册 / 找回密码 / 注销三种发码的对外行为（含防枚举静默）与改造前完全一致。
4. 「验证码」术语写入 CONTEXT.md。