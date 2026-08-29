# 邮箱即主键的注册与账号模型

将注册与账号标识从「用户名」改为「邮箱为主」，缩短注册信息，增强账号安全，
并添加发码与登录限流。本 ADR 更迭 ADR-0001 中「账号体系」与「密码管理」两节
的决定，ADR-0001 的「响应信封」「双 token」部分不变。

## 现状

ADR-0001 确立的账号标识：

* 主标识为 **username**（注册必填、唯一），`email` 仅作为找回密码通道

* 注册入参 `{username, password, email}`，注册即返回双 token

* 找回密码以 `{username, email}` 校验

## 动机

### 简化注册

移动阅读 App 期望「注册要快」。让用户填用户名不是必须的——用户名是展示性标识，
可以后期自改；真正的身份凭据应是最易表达、且能证明所有权的东西。业界（尤其欧美
SaaS）多以「邮箱 + 验证邮件」确认账号所有权。

### 邮箱即所有权证明

用「邮箱 + 邮箱验证码」注册，能同时完成「创建账号」与「证明邮箱归我」：

* 无需单独的激活流程——注册成功即邮箱已验证

* 避免注册出「永远无法找回密码」的死账号（邮箱填错/填别人的情况）

### 防暴力

注册/登录/找回都围绕邮箱展开，邮箱便成了可枚举的攻击面。需要为发码与登录
增加限流，避免验证码暴力枚举与密码撞库。

## 决策

### 1. 账号标识与约束

| 字段         | 约束                  | 说明              |
| ---------- | ------------------- | --------------- |
| `uid`      | 主键，自增               | 账号根标识，永不改变      |
| `email`    | 唯一、非空               | **登录主标识**       |
| `username` | 非空、但**可重复**（去掉唯一索引） | 展示用，注册时自动生成，可后改 |
| `nickname` | 可有                  | 展示用             |
| `avatar`   | 可有                  | <br />          |
| `password` | 非空（bcrypt）          | <br />          |

* `users` 表约束变更：`email` 加唯一索引；`username` 移除唯一索引、保留 `NOT NULL`。

* 注册时后端为 `username` 生成随机占位（如 `user_<8位随机>`），用户可在「修改资料」中自改。

### 2. 注册三接口（每个端点职责单一）

| 方法   | 路径                  | 入参                        | 成功后返回                       |
| ---- | ------------------- | ------------------------- | --------------------------- |
| POST | /api/auth/send-code | `{email}`                 | `00000`（不发码之外无副作用）          |
| POST | /api/auth/register  | `{email, code, password}` | `00000`（激活并建号，**不发 token**） |
| POST | /api/auth/login     | `{email, password}`       | `00000` + 双 token           |

* `send-code`：按 `email` 生成 6 位验证码并发送，不改账号状态。

* `register`：校验验证码，通过则创建已激活账号（`email` 即已确认），**不发 token**。

* `login`：校验账号存在 + 密码正确，返回双 token——**只有 login 发 token**。

登录「用户**需主动登录**」，注册只是注册，不自动登录。

### 3. 找回密码（纯邮箱）

失去用户名标识后，找回密码不再依赖 `username`，完全走邮箱：

| 方法   | 路径                                  | 入参                            |
| ---- | ----------------------------------- | ----------------------------- |
| POST | /api/auth/forgot-password/send-code | `{email}`                     |
| POST | /api/auth/forgot-password/reset     | `{email, code, new_password}` |

### 4. 限流（防暴力）

* **发码限流**：同一 `email` 频率受限（默认每分钟至多 1 次、每小时至多 5 次），
  命中返回 `A0241`。验证码防暴力依赖此发码闸门，而非单纯的尝试次数。

* **登录限流**：同一 `email` 连续登录失败 5 次锁定 15 分钟（`login_attempts` +
  `locked_until` 字段），锁定期间即使密码正确也拒绝并返回剩余锁定时间，到点自动解锁。

### 5. 移除的接口

ADR-0001 计划的 `verify-code`、`resend-code` 不再需要——注册即激活，激活不再
独立成步。旧注册形态（`{username, password, email}`）被 `{email, code, password}` 取代。

## 被拒方案

* **用户名仍作登录主标识**：与「简化注册」「注册不填用户名」的目标冲突。

* **注册后自动登录（register 返回 token）**：会让「register 也能发 token」，
  稀释「仅 login 发 token」的语义；注册与登录的职责边界也随之模糊。

* **发验证码后单独激活流程**：多一个「未激活」状态与对应接口（`verify-code`/
  `resend-code`），增加联动复杂度；注册即激活更简洁。

## 下游影响

* `model/user.go`：主键改名 `uid`；`email` 唯一必填；`username` 去唯一索引、非空、
  保留 `nickname`/`avatar`；新增 `login_attempts`、`locked_until`。

* `service/auth.go` / `handler/auth.go`：实现 `send-code`、新 `register`、`login`、
  `forgot-password/*`；将查找账号由 `FindByUsername` 改为 `FindByEmail`。

* `main.go`：注册 `send-code`，移除 `verify-code`/`resend-code`。

* `sql/init.sql`：`users.email` 唯一、`users.username` 去唯一索引、加限流字段。

* 测试与文档同步更新。

## 验收标准

1. `go test ./...` 全量通过。
2. curl 走通：`send-code` → `register(email,code,password)` → `login(email,password)`。
3. 只有 `login` 返回 token；`register` 返回但不带 token。
4. 同一 `email` 发码频率受限；登录失败 5 次锁定 15 分钟。
5. `forgot-password/*` 全程用邮箱，不依赖用户名。

