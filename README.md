# ebook-server

[![CI](https://github.com/xrn1997/ebook-server/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/xrn1997/ebook-server/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

> **小说阅读器后端服务** —— 为 Android 客户端 [android-ebook](https://github.com/xrn1997/android-ebook)
> 提供账号、认证、评论与操作日志 API，自带 Vue 后台管理界面。Go + Gin + SQLite，构建后后台前端
> 内嵌进单一可执行文件（前后端 monorepo，[ADR-0009](docs/adr/0009-monorepo-backend-and-admin-frontend.md)）。

- 许可：[Apache-2.0](LICENSE) · Go 1.26 · 数据库零配置（SQLite 单文件，版本化迁移）
- 交付物：一个二进制 + `config.yaml`（可选 `.env`），无需另起前端服务或数据库
- 公开 API（`:9090`）与后台管理（`:9091`）是两个独立监听面，默认都只监听 `127.0.0.1`

## 核心特性

- **账号与认证**：邮箱 + 验证码注册；登录返回双 token（access + refresh，刷新即轮换）；改密/注销需二次确认；注销是匿名化而非删除，公开评论保留
- **评论**：按「评论聚合键」聚合成桶，键由客户端从「书名 + 作者」派生、服务端只存不解释；支持书籍级/章级评论、本人改键迁移与删除
- **操作日志**：客户端业务请求自动审计，本人可读；跨用户审计读取只在后台表面
- **后台管理**：用户 / 评论 / 日志管理与统计；独立 JWT 鉴权、独立监听，默认公网不可达
- **单机可部署**：Nginx 终止 TLS 后反代，systemd 托管；Nginx / systemd / logrotate 模板见 [deploy/README.md](deploy/README.md)

## 快速开始

环境：Go 1.26+；Node.js `^20.19.0 || ^22.13.0 || >=24`（仅构建/检查后台前端需要）；
Windows 构建服务端 exe 需要 MinGW-w64（提供 `windres`）。无需安装数据库。

```bash
git clone <repository-url> && cd ebook-server
cd backend && go mod tidy && cd ..
make all
./build/ebook-server   # Windows 为 build/ebook-server.exe
```

`make all` 先构建后台前端、再内嵌进 Go 二进制，这样 `/admin` 才是完整后台。
**服务必须从仓库根启动**：`config.yaml`、`ebook.db`、`uploads/` 都按工作目录解析；
首次运行自动创建数据库文件与上传目录。构建、测试、发版命令全集见 [docs/development.md](docs/development.md)。

## API 接口

**统一响应信封**：业务端点 HTTP 状态码恒为 200，以 `code` 业务码区分成败（`"00000"` 为成功），
错误文案在 `error` 字段，可直达移动端 Toast；无载荷的成功不带 `data`（[ADR-0001](docs/adr/0001-response-envelope-and-auth-model.md)）。
完整请求/响应结构以 Swagger UI 为准：公开端口默认关闭（`api_docs.enabled`，防接口清单泄露），
后台端口 `http://127.0.0.1:9091/api-docs/` 始终提供。

### 服务自身

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /health | 健康检查，不入操作审计 | 否 |
| GET | /api-docs/ | Swagger UI + OpenAPI spec（公开端口默认关闭，后台端口始终提供） | 否 |

### 认证相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | /api/auth/send-code | 按邮箱发送验证码 | 否 |
| POST | /api/auth/register | 注册（邮箱+验证码+密码），激活建号，不发 token | 否 |
| POST | /api/auth/login | 邮箱+密码登录，返回双 token | 否 |
| POST | /api/auth/refresh | 刷新 token（轮换） | 否（用 refresh token） |
| POST | /api/auth/logout | 登出 | 是 |
| POST | /api/auth/forgot-password/send-code | 忘记密码发邮箱验证码 | 否 |
| POST | /api/auth/forgot-password/reset | 验证码重置密码 | 否 |

> **账号模型**：`email` 是登录主标识（唯一必填、注册即定不可变），`uid` 是账号根标识，
> 展示用名非唯一、可改（[ADR-0002](docs/adr/0002-email-based-registration-and-account-model.md)、[CONTEXT.md](CONTEXT.md)）。
>
> **验证码错误码**三个流程共用一套判定（[ADR-0008](docs/adr/0008-verification-code-sender-module.md)）：
> `A0132` 验证码错误或已过期（含没发过、已消费、跨流程使用）；`A0241` 次数超限
> （发码限流：每邮箱每流程每分钟至多 1 次、每小时至多 5 次；或校验累计错满 5 次）。

### 用户相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/users/me | 获取当前用户信息 | 是 |
| PUT | /api/users/me | 更新用户信息（`email` 不可变更，传不同值返回 `A0113`） | 是 |
| PUT | /api/users/me/password | 已登录修改密码（旧密码二次确认） | 是 |
| GET | /api/users/me/data | 导出我的数据（用户资料 + 本人评论） | 是 |
| POST | /api/users/me/deletion/send-code | 发送注销验证码到当前账号邮箱 | 是 |
| POST | /api/users/me/deletion | 注销账号（验证码确认，匿名化并返回数据副本） | 是 |

> **账号注销**（[ADR-0005](docs/adr/0005-account-deletion-by-anonymization.md)）：匿名化而非删除，
> **不可撤销**；同一邮箱可重新注册；公开评论与操作日志保留，评论作者显示「已注销用户」。
> 注销前先 `GET /api/users/me/data` 导出，注销响应也带同一份副本。

### 评论相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/comments | 评论列表，按 `comment_keys`（逗号分隔聚合键）过滤并返回并集 | 否 |
| POST | /api/comments | 创建评论，`comment_key` **必填**，`chapter_name`/`book_name` 为可选展示快照 | 是 |
| GET | /api/comments/my | 获取我的评论 | 是 |
| POST | /api/comments/migrate | 迁移本人评论的聚合键（旧键→新键，同键返回 `A0305`） | 是 |
| DELETE | /api/comments/:id | 删除评论（仅本人，非本人返回 `A0303`） | 是 |

> **评论聚合键**（[ADR-0014](docs/adr/0014-comment-key-switch-to-client-derived-token.md)、[CONTEXT.md](CONTEXT.md)）：
> `comment_key` 由客户端从「书名 + 作者」派生（`ck1:` + sha256，章评追加 `#章序号`，作品键本身即书籍级评论），
> 服务端**只存不解释**；创建缺键回 `A0400`。`comment_key` 为空的是换键前的历史行（不是书籍级评论），
> 只能靠已废弃的 `chapter_url` 参数读出；`chapter_url`/`book_name` 仅为兼容读路径与展示快照。
> 评论域错误码：`A0303` 无权删除、`A0304` 评论不存在、`A0305` 聚合键与旧键相同。
> 响应 `user` 只含 `uid/username/nickname/avatar`（不含 email）；`add_time` 为上海时区
> `yyyy-MM-dd HH:mm:ss`；列表为分页包裹 `{items, total, page, page_size}`，按 `add_time` 倒序。
> 后台另有跨用户改键 `/admin/api/comments/rehash`（运维修键，管理页面不提供按钮）。

### 头像上传

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | /api/uploads/avatar | 上传头像（multipart 字段名 `avatar`，jpg/png/webp ≤5MB），返回图片 URL | 是 |
| GET | /uploads/avatar/* | 头像静态访问（只放行 `upload.dir` 下的 avatar 子目录） | 否 |

> **两步提交**（ADR-0011）：先上传拿 `{url}`，再 `PUT /api/users/me` 携带 `{"avatar": "<url>"}`
> 完成更新；更换头像时旧文件在没有任何账号引用它时自动清理。

### 日志相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/logs/my | 获取我的操作日志 | 是 |

## 请求示例

登录（返回双 token）：

```bash
curl -X POST http://localhost:9090/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com", "password": "123456"}'
```

```json
{ "code": "00000", "error": "",
  "data": { "token": "eyJ...", "refresh_token": "a1b2c3...",
            "user": { "uid": 1, "email": "test@example.com", "username": "user_1a2b3c4d", "nickname": "user_1a2b3c4d", "avatar": "" } } }
```

创建评论（`comment_key` 必填；作品键即书籍级评论，作品键 + `#章序号` 即章评）：

```bash
curl -X POST http://localhost:9090/api/comments \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{"content": "这是一条评论", "comment_key": "ck1:3a7f...e91"}'
```

查询评论（`comment_keys` 逗号分隔传多个键，返回并集）：
`curl "http://localhost:9090/api/comments?comment_keys=ck1%3A3a7f...e91%234&page=1&page_size=20"`

上传头像（两步提交）：

```bash
curl -X POST http://localhost:9090/api/uploads/avatar \
  -H "Authorization: Bearer <your-token>" \
  -F "avatar=@/path/to/avatar.png"
# 返回 {"url": "http://<host>:9090/uploads/avatar/xxxx.png"}，随后提交到用户资料：
curl -X PUT http://localhost:9090/api/users/me \
  -H "Content-Type: application/json" -H "Authorization: Bearer <your-token>" \
  -d '{"avatar": "http://<host>:9090/uploads/avatar/xxxx.png"}'
```

## 后台管理系统

后台管理（Vue 前端 + API）挂 `/admin`，与公开 API 完全分离（[ADR-0009](docs/adr/0009-monorepo-backend-and-admin-frontend.md)）：

- **独立鉴权**：管理端 token 用独立的 `admin.jwt_secret` 签发，与用户 token 互不可用
- **网络隔离**（[ADR-0010](docs/adr/0010-admin-network-isolation.md)）：后台独立监听
  `127.0.0.1:9091`，公网物理上无法连接；远程管理走 SSH 隧道 / VPN
- **登录限流**：失败尝试 5 次/分钟/来源 IP，超限 `A0241`；本机访问 `http://127.0.0.1:9091/admin`
  （账号见 `admin.username`，密码来自 `config.yaml` 或 `.env` 的 `ADMIN_PASSWORD`）
- **会话**：管理 token 存浏览器 `localStorage`，只用于同源 `/admin/api` 调用，鉴权失败即清除并回登录页
- **能力边界**：用户搜索/详情、评论搜索/删除（两步确认）、日志筛选，列表均支持分页；
  配置与服务控制不在页面里（`config.yaml` / `.env` 由部署者维护）
- 后台引擎自带 Swagger 文档（`http://127.0.0.1:9091/api-docs/`）

### 后台管理 API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /admin/api/login | 后台登录，返回管理端 token |
| GET | /admin/api/stats | 用户数 + 评论数 |
| GET | /admin/api/users | 用户列表（分页；`keyword` 模糊匹配邮箱/用户名/昵称，纯数字兼命中 UID） |
| GET | /admin/api/users/:uid | 用户详情（资料 + 该用户评论数） |
| GET | /admin/api/comments | 评论列表（分页；`keyword` 内容模糊、`book_name` 精确过滤） |
| DELETE | /admin/api/comments/:id | 删除评论（后台治理，软删除，不做归属校验） |
| POST | /admin/api/comments/rehash | 全局改评论聚合键（跨用户，空旧键一律拒绝 `A0400`，同键 `A0305`） |
| GET | /admin/api/logs | 操作日志（分页；`method`/`path`/`user_id`/`error_code`/`failed` 筛选） |

> 评论列表返回固定视图 `AdminCommentView`：作者只含 `uid`/`username`/`nickname`/`avatar`，
> 不含 email；后台治理删与作者删共用同一删除实现，只差归属范围（[ADR-0015](docs/adr/0015-comment-scope-and-admin-view.md)）。

## 配置说明

配置文件 `config.yaml`：

```yaml
server:
  port: 9090            # 服务端口
  listen_addr: 127.0.0.1  # 仅回环；公网入口由 Nginx 反代
  mode: debug           # 运行模式: debug, release, test（生产用 SERVER_MODE=release 覆盖）
  public_base_url: ""   # 头像等公开资源的来源，如 https://book.example.com；留空按请求 Host 拼接

database:
  path: ebook.db        # SQLite 数据库文件路径

api_docs:
  enabled: false        # 公开端口是否提供 Swagger（默认关；spec 是全量的，开启会连后台管理端点清单一并暴露，仅限受控网络临时开启）

jwt:
  secret: your-secret-key-change-this  # JWT 密钥（请修改）
  expire_min: 120       # access token 过期时间（分钟）

admin:
  username: admin
  password: change-me-admin            # 真实值放 .env 的 ADMIN_PASSWORD
  jwt_secret: your-admin-secret-change-this  # .env 的 ADMIN_JWT_SECRET 覆盖
  expire_min: 60
  listen_addr: 127.0.0.1  # 后台监听地址：默认仅本机
  listen_port: 9091       # 后台监听端口（与公开 API 的 9090 分离）

smtp:
  host: smtp.example.com  # 非 release 模式下留空则验证码写日志
  port: 465               # 465 走 TLS；587 走 STARTTLS
  username: no-reply@example.com
  password: change-me     # 真实授权码放 .env 的 SMTP_PASSWORD
  from: no-reply@example.com
  insecure: false

upload:
  dir: uploads            # 上传文件根目录（仅 avatar/ 子目录经 /uploads/avatar/* 公开）
```

> 敏感项走本地 `.env`（已被 git 排除，见 `.env.example`）：`JWT_SECRET`、`SMTP_PASSWORD`、
> `ADMIN_PASSWORD`、`ADMIN_JWT_SECRET` 覆盖对应 YAML 项；对外部署再加 `SERVER_MODE=release`
> 与 `SERVER_PUBLIC_BASE_URL=https://<域名>`（否则头像 URL 拼成 `http://`）。
>
> **不安全的密钥配置会被启动拒绝**：`jwt.secret` / `admin.jwt_secret` / `upload.dir` 留空
> 一律拒绝启动，`mode: release` 下还拒绝内置默认值；配置文件里的显式空值**优先于**代码默认值。
> 环境变量优先于 YAML。配置契约的唯一来源是 `backend/config`：字段清单来自
> `config.go` 的 `mapstructure` 标签，默认值、环境变量覆盖与校验规则只住
> `schema.go`（[ADR-0016](docs/adr/0016-config-schema-generated-from-go.md)）；
> 配置只由部署者维护，不经过浏览器后台。

## 部署

部署只需要二进制、`config.yaml` 与可选的 `.env`，从放有 `config.yaml` 的工作目录启动：

```bash
make all
./build/ebook-server   # Windows 为 build/ebook-server.exe
```

公开 API 默认只监听 `127.0.0.1:9090`：公网入口由 Nginx 终止 TLS 后反代，应用端口不直接暴露；
后台默认只监听 `127.0.0.1:9091`，远程管理走 SSH 隧道/VPN。生产部署 runbook（拓扑、配置、
Nginx/systemd/logrotate 模板、审计清理、备份与上线验证清单）见 [deploy/README.md](deploy/README.md)。

**不经 Nginx 直连**（本机之外的设备、Android 真机联调等）需显式设 `server.listen_addr: 0.0.0.0`
或 `SERVER_LISTEN_ADDR=0.0.0.0`，并自行承担明文传输与限流缺失（[ADR-0027](docs/adr/0027-public-api-loopback-and-trusted-proxy.md)）。

## 开发与文档

```bash
make gates   # 提交门总入口：lint + test-race + 前端三门，与 CI 同一条命令
```

更多工程细节：[开发手册](docs/development.md) · [架构与形状](docs/architecture.md) ·
[领域词汇](CONTEXT.md) · [测试策略](TESTING.md) · [贡献指南](CONTRIBUTING.md) ·
[决策记录](docs/adr/README.md) · [文档规范](docs/AGENTS.md)

## 贡献与许可

欢迎提交 Issue 与 PR，参与前请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。

主体代码基于 [Apache-2.0](LICENSE) 协议开源。
