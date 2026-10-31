# ebook-server

一个基于 Go + Gin 的轻量级小说阅读器后端服务，配套 Android 客户端
[android-ebook](https://github.com/xrn1997/android-ebook)。仓库为
**前后端 monorepo**（[ADR-0009](docs/adr/0009-monorepo-backend-and-admin-frontend.md)）：
Go 后端 + Vue 后台管理前端，构建后前端内嵌进单一可执行文件。

## 技术栈

- **语言**: Go 1.23
- **Web 框架**: Gin
- **ORM**: GORM + SQLite（glebarez/sqlite，纯 Go 驱动，零配置）
- **认证**: JWT（用户 token 与后台管理 token 相互隔离）
- **配置**: Viper（YAML + `.env` 覆盖）
- **日志**: Zap
- **数据库**: SQLite（单文件数据库，版本化迁移）
- **后台前端**: Vue 3 + Vite（构建产物 go:embed 进单 exe）

## 项目结构

- `backend/` — Go 后端（公开 API、后台 API、业务与数据层）
- `frontend/` — 后台管理前端（构建产物内嵌进 Go 二进制）
- `docs/` — 架构、ADR、开发手册与文档规范
- `sql/` — 数据库参考脚本（实际使用 SQLite）

分层、依赖方向与每条语义的主人见 [docs/architecture.md](docs/architecture.md)；构建与测试命令见 [docs/development.md](docs/development.md)。

## 快速开始

### 环境要求

- Go 1.23+
- Node.js 20+（仅构建后台前端时需要）
- Windows 构建服务端 exe 时需要 MinGW-w64（提供 `windres`；WinLibs 自带）
- 无需安装数据库（SQLite 自动创建）

### 安装步骤

1. **克隆项目**
   ```bash
   git clone <repository-url>
   cd ebook-server
   ```

2. **安装依赖**
   ```bash
   cd backend && go mod tidy
   cd ..
   ```

3. **构建并运行项目**
   ```bash
   make all
   ./build/ebook-server   # Windows 为 build/ebook-server.exe
   ```

   `make all` 会先构建后台前端，再把产物内嵌进 Go 二进制；这样 `/admin` 才是完整后台。
   只需要公开 API 时可改用 `make run`，但它在未执行 `make all` 时只提供占位页。

   必须从仓库根启动：`main.go` 按相对路径读 `config.yaml`，而该文件（以及 `ebook.db`、
   `uploads/`）都落在工作目录下——在 `backend/` 里 `go run main.go` 会找不到配置。

   首次运行会自动创建 `ebook.db` 数据库文件与 `uploads/` 目录。

### 使用 Makefile

```bash
make all      # 前端 build + 后端 build，产出内嵌后台前端的单 exe
make build    # 仅构建 Go 后端（复用已有的 frontend/dist）
make run      # 在 backend/ 构建到 build/，再从仓库根启动
make docs     # 重新生成 Swagger 文档（swag init，需先安装 swag）
make frontend-test  # 后台前端类型检查 + vitest
make clean    # 清理
```

## API 接口

### 服务自身

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /health | 健康检查，无需认证、不入操作审计 | 否 |
| GET | /api-docs/ | Swagger UI + OpenAPI spec。**公开端口默认关闭**（`api_docs.enabled`，防接口清单泄露）；后台端口 9091 始终提供 | 否 |

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

> **账号模型**：`email` 是登录主标识（唯一必填），`uid` 是主键，`username` 非空但
> 可重复（注册时自动生成，可后改）。详见 [ADR-0002](docs/adr/0002-email-based-registration-and-account-model.md)。
>
> **验证码错误码**：注册 / 找回密码 / 注销三个流程共用一套判定，下发与校验同源于
> `VerificationCodes` 模块（[ADR-0008](docs/adr/0008-verification-code-sender-module.md)）。
> `A0132` 验证码错误或已过期（含没发过、已消费、跨流程使用）；`A0241` 尝试次数超限
> ——既指发码限流（每邮箱每流程 1 次/分钟、5 次/小时），也指校验累计错满 5 次，
> 三个用验证码的端点都会回它。

### 用户相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/users/me | 获取当前用户信息 | 是 |
| PUT | /api/users/me | 更新用户信息（`email` 不可变更，传不同值返回 `A0113`） | 是 |
| PUT | /api/users/me/password | 已登录修改密码 | 是 |
| GET | /api/users/me/data | 导出我的数据（用户资料 + 本人评论） | 是 |
| POST | /api/users/me/deletion/send-code | 发送注销验证码到当前账号邮箱 | 是 |
| POST | /api/users/me/deletion | 注销账号（验证码确认，匿名化并返回数据副本） | 是 |

> **账号注销说明**（[ADR-0005](docs/adr/0005-account-deletion-by-anonymization.md)）
>
> * 注销采用**匿名化**而非删除：改写 `email` 为占位值、清空密码与头像（头像文件尽力清理，
>   仍被其他账号引用的文件保留），账号不可再登录
> * 同一邮箱注销后可以**重新注册**（占位 email 释放了唯一索引）
> * **公开评论与操作日志保留**——评论作者将显示为"已注销用户"
> * **不可撤销**：注销前请先用 `GET /api/users/me/data` 导出数据，注销响应也会带上同一份副本

### 评论相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/comments | 评论列表，按 `comment_keys`（逗号分隔的聚合键）过滤并返回并集；`chapter_url`/`book_name` 已废弃，仅兼容旧数据 | 否 |
| POST | /api/comments | 创建评论，`comment_key` **必填**，`chapter_name`/`book_name` 为可选展示快照 | 是 |
| GET | /api/comments/my | 获取我的评论 | 是 |
| POST | /api/comments/migrate | 迁移自己评论的聚合键（旧键→新键，作用列 `comment_key`，仅本人；同键返回 `A0305`） | 是 |
| DELETE | /api/comments/:id | 删除评论（仅本人，非本人返回 `A0303`） | 是 |

> **评论聚合键**（[ADR-0011](docs/adr/0011-comment-chapter-and-avatar-upload.md) →
> [ADR-0014](docs/adr/0014-comment-key-switch-to-client-derived-token.md)）：
> 聚合键 `comment_key` 由**客户端**从「书名+作者」派生（`ck1:` + sha256，章评追加 `#章序号`），
> 后端**只存不解释**：不校验格式、不建书籍表、不提供「列出所有书」。创建时缺键直接 `A0400`——
> 缺键的评论会落进任何查询都命中不到的空桶。`chapter_url`/`book_name` 已废弃（前者仅作历史行
> 的兼容读路径，两者都只是展示快照）；`comment_key` 为空串代表换轨前的历史行，不是书籍级评论。
> 评论响应用独立视图：`user` 只含 `uid/username/nickname/avatar`（不含 email），
> `add_time` 固定上海时区 `yyyy-MM-dd HH:mm:ss` 格式。
> 评论域错误码：`A0303` 无权删除、`A0304` 评论不存在、`A0305` 聚合键与旧键相同（`A0301`/`A0302` 预留）。
> 键的规则只有一处实现：公开的 `/api/comments/migrate`（改**本人**行）与后台
> `/admin/api/comments/rehash`（改**全员**行）只差[改键归属范围](CONTEXT.md)，空目标键、
> 全员范围内的空旧键、超过 200 字节的键都回 `A0400`；列表的 `comment_keys`/`chapter_url`
> 过滤同样以 `A0400` 挡住超长与超数（上限 50 个键、单键 200/2048 字节）。
> 后台的 rehash 只有 API：管理页面不提供改键按钮，跨用户修键是运维动作，不是日常治理动作。

### 头像上传

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | /api/uploads/avatar | 上传头像（multipart，字段名 `avatar`，jpg/png/webp ≤5MB），返回图片 URL | 是 |
| GET | /uploads/avatar/* | 头像的静态访问（只挂头像子目录，upload.dir 下的其他文件不放出去） | 否 |

> **头像两步提交**（ADR-0011）：先 `POST /api/uploads/avatar` 拿到 `{url}`，
> 再 `PUT /api/users/me` 携带 `{"avatar": "<url>"}` 完成更新；更换头像时，旧文件在没有任何
> 账号的头像仍指向它时自动清理（引用按 URL 的文件名判定，host 写法不同也算同一个文件）。

### 日志相关

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | /api/logs/my | 获取我的操作日志 | 是 |

## 后台管理系统

后台管理（Vue 前端 + API）挂 `/admin`，与公开 API **完全分离**（[ADR-0009](docs/adr/0009-monorepo-backend-and-admin-frontend.md)）：

- **独立鉴权**：管理端 token 用独立的 `admin.jwt_secret` 签发，与用户 token 互不可用
- **网络隔离**（[ADR-0010](docs/adr/0010-admin-network-isolation.md)）：后台独立监听
  `admin.listen_addr:admin.listen_port`（默认 `127.0.0.1:9091`），**公网物理上无法连接**；
  远程管理请走 SSH 隧道 / VPN，不要直接开放后台端口
- **登录限流**：后台登录失败尝试 5 次/分钟/来源 IP，超限返回 `A0241`；成功登录不消耗配额
- 本机访问：`http://127.0.0.1:9091/admin`（账号见 `admin.username`，密码来自 `config.yaml` 或 `.env` 的 `ADMIN_PASSWORD`；生产必须修改）
- 后台引擎自带 Swagger 文档（`http://127.0.0.1:9091/api-docs/`），供本机查阅

### 后台管理 API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /admin/api/login | 后台登录（失败尝试按来源 IP 限流 5 次/分钟，超限 `A0241`），返回管理端 token |
| GET | /admin/api/stats | 用户数 + 评论数 |
| GET | /admin/api/users | 用户列表（分页；`keyword` 模糊匹配邮箱/用户名/昵称，纯数字兼命中 UID） |
| GET | /admin/api/users/:uid | 用户详情（资料 + 该用户评论数） |
| GET | /admin/api/comments | 评论列表（分页；`keyword` 内容模糊、`book_name` 精确过滤） |
| DELETE | /admin/api/comments/:id | 删除评论（后台治理，软删除，不做归属校验） |
| POST | /admin/api/comments/rehash | 全局改评论聚合键（跨用户，旧键→新键；空旧键一律拒绝 `A0400`，同键 `A0305`） |
| GET | /admin/api/logs | 操作日志（分页；`method`/`path`/`user_id`/`error_code`/`failed` 筛选） |

> 评论列表返回固定视图 `AdminCommentView`：作者只含 `uid`/`username`/`nickname`/`avatar`，
> **不含 email**，也不直接序列化评论实体。后台治理删与作者删共用同一删除实现，只差归属范围
> （[ADR-0015](docs/adr/0015-comment-scope-and-admin-view.md)）。

### 内嵌 web 后台

后台页面随二进制一起交付，与后台 API 同源（`/admin/api/*`），无需另起前端服务：

- **完整管理能力**（[ADR-0013](docs/adr/0013-comment-key-migration-and-admin-api.md)）：
  用户搜索与详情、评论搜索与删除（两步确认）、日志筛选，全部列表支持分页
- **会话**：管理 token 存浏览器 `localStorage`，只用于同源 `/admin/api` 调用；
  鉴权失败自动清除并回登录页
- **配置与服务控制不在页面里**：`config.yaml` / `.env` 由部署者维护，进程交给 systemd
  或直接启动；后台只做数据管理，不托管密钥
- 开发/测试：`make frontend-dev` / `make frontend-test`；构建：`make all`

## 请求示例

> **统一响应信封**：所有业务端点 HTTP 状态码恒为 200，通过 `code` 业务码区分成败
> （`"00000"` 表示成功），错误文案位于 `error` 字段，可直达移动端 Toast。

### 发送验证码

前端点击「发送验证码」时调用：

```bash
curl -X POST http://localhost:9090/api/auth/send-code \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com"}'
```

### 注册

```bash
curl -X POST http://localhost:9090/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "code": "123456",
    "password": "123456"
  }'
```

注册成功（激活建号，**不发 token**），返回 `code 00000`。

### 登录

```bash
curl -X POST http://localhost:9090/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "123456"
  }'
```

登录成功（返回双 token）：

```json
{
  "code": "00000",
  "error": "",
  "data": {
    "token": "eyJ...",
    "refresh_token": "a1b2c3...",
    "user": {
      "uid": 1,
      "email": "test@example.com",
      "username": "user_1a2b3c4d",
      "nickname": "user_1a2b3c4d",
      "avatar": ""
    }
  }
}
```

### 刷新 token

```bash
curl -X POST http://localhost:9090/api/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "a1b2c3..."}'
```

### 已登录修改密码

```bash
curl -X PUT http://localhost:9090/api/users/me/password \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access-token>" \
  -d '{"old_password": "123456", "new_password": "654321"}'
```

### 获取用户信息

```bash
curl http://localhost:9090/api/users/me \
  -H "Authorization: Bearer <your-token>"
```

### 创建评论

`comment_key` 必填。作品键（不含 `#`）即书籍级评论：

```bash
curl -X POST http://localhost:9090/api/comments \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "content": "这是一条评论",
    "comment_key": "ck1:3a7f...e91"
  }'
```

章评键在作品键后追加 `#章序号`，展示快照可选：

```bash
curl -X POST http://localhost:9090/api/comments \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{
    "content": "这一章的剧情真精彩",
    "comment_key": "ck1:3a7f...e91#4",
    "chapter_name": "第五章",
    "book_name": "示例之书"
  }'
```

### 查询评论

```bash
curl "http://localhost:9090/api/comments?comment_keys=ck1%3A3a7f...e91%234&page=1&page_size=20"
```

> 列表统一返回分页包裹结构 `{items, total, page, page_size}`，按 `add_time` 倒序。

`comment_keys` 用逗号传多个键，返回这些桶评论的**并集**（跨书源合并同一作品）：

```bash
curl "http://localhost:9090/api/comments?comment_keys=ck1:aaa%234,ck1:bbb%234"
```

> 已废弃的 `chapter_url`（可重复传）与 `book_name` 仍可用于读取换轨前的历史行，
> 但新客户端不应依赖：后端算不出这些行对应的 `comment_key`（派生需要作者，而作者从未入库）。

### 上传头像（两步提交）

第一步上传拿 URL：

```bash
curl -X POST http://localhost:9090/api/uploads/avatar \
  -H "Authorization: Bearer <your-token>" \
  -F "avatar=@/path/to/avatar.png"
```

返回 `{"url": "http://<host>:9090/uploads/avatar/xxxx.png"}`，第二步提交到用户资料：

```bash
curl -X PUT http://localhost:9090/api/users/me \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{"avatar": "http://<host>:9090/uploads/avatar/xxxx.png"}'
```

## 部署

后台前端已内嵌在二进制里，部署只需要二进制、`config.yaml` 与可选的 `.env`：

```bash
make all
./build/ebook-server   # Windows 为 build/ebook-server.exe
```

服务必须从放有 `config.yaml` 的工作目录启动；`ebook.db`、`uploads/`、`logs/` 都相对该目录。
交付物只有这一个二进制与配置文件：**不提供容器镜像**，进程由 systemd 或其它进程管理器
托管，把工作目录设为部署目录即可。后台默认只监听
`127.0.0.1:9091`，远程管理请走 SSH 隧道/VPN，不要改成通配监听。

## 配置说明

配置文件 `config.yaml`：

```yaml
server:
  port: 9090            # 服务端口（config.yaml 与代码默认值一致，均为 9090）
  mode: debug           # 运行模式: debug, release, test
  public_base_url: ""   # 头像等公开资源的来源，如 https://book.example.com；留空则按请求 Host 拼接（仅限受信网络，见 ADR-0011）

database:
  path: ebook.db      # SQLite 数据库文件路径

api_docs:
  enabled: false      # 公开端口是否提供 Swagger 文档（默认关，防接口清单泄露；开启后会连后台管理端点清单一并展示，仅限受控网络临时开启）

jwt:
  secret: your-secret-key-change-this  # JWT 密钥（请修改）
  expire_min: 120          # access token 过期时间（分钟）

admin:
  username: admin            # 后台管理系统账号
  password: change-me-admin  # 后台密码（真实值放 .env 的 ADMIN_PASSWORD）
  jwt_secret: your-admin-secret-change-this  # 管理端 JWT 密钥（.env 的 ADMIN_JWT_SECRET 覆盖）
  expire_min: 60             # 管理端 token 有效期（分钟）
  listen_addr: 127.0.0.1     # 后台监听地址：默认仅本机（公网不可达）；局域网访问改内网 IP
  listen_port: 9091          # 后台监听端口（与公开 API 的 9090 分离）

smtp:
  host: smtp.example.com   # SMTP 服务器（非 release 模式下留空则验证码写日志）
  port: 465                # 465 走 TLS；587 走 STARTTLS
  username: no-reply@example.com
  password: change-me      # 真实授权码放 .env 的 SMTP_PASSWORD
  from: no-reply@example.com
  insecure: false

upload:
  dir: uploads             # 上传文件根目录（头像存 <dir>/avatar/，仅该子目录经 /uploads/avatar/* 公开）
```

> 敏感项走本地 `.env`（已被 git 排除）：`JWT_SECRET` 覆盖 `jwt.secret`，`SMTP_PASSWORD` 覆盖
> `smtp.password`，`ADMIN_PASSWORD` 覆盖 `admin.password`，`ADMIN_JWT_SECRET` 覆盖
> `admin.jwt_secret`。见 `.env.example`。
>
> **不安全的密钥配置会在启动时被拒绝**：`jwt.secret` / `admin.jwt_secret` / `upload.dir`
> 留空一律拒绝启动，`mode: release` 下还拒绝代码内置的默认值。注意配置文件里的显式空值
> **优先于**代码默认值——留空不等于「用默认的」，那会把 HMAC 密钥变成空串、任何人都能给自己
> 签一个任意用户的 access token。环境变量（`JWT_SECRET` / `ADMIN_JWT_SECRET` / `ADMIN_PASSWORD`）
> 优先于 YAML。
>
> **配置契约只有一份**：字段来自 `backend/config/config.go` 的 `mapstructure` 标签，
> 默认值、环境变量覆盖与校验规则来自 `backend/config/schema.go`；`config.Load` 在启动时执行
> 完整校验（空密钥、非法端口、通配后台监听等直接拒绝）。配置只由部署者通过 YAML/.env
> 维护，不经过浏览器后台。

## 开发说明

添加新接口的标准流程、依赖方向与测试门禁见 [docs/development.md](docs/development.md) 与 [docs/architecture.md](docs/architecture.md)。

数据库使用版本化迁移（`backend/migrations/`）：新库启动时执行 SQL 基线，AutoMigrate 时代的旧库会自动收养。新增 schema 变更见 [docs/development.md](docs/development.md) 的「数据库迁移」。

## 测试

```bash
cd backend && go test ./...
make frontend-test   # 后台前端类型检查 + vitest
```

完整的测试策略、测试结构、覆盖率口径、前端测试见 [TESTING.md](TESTING.md)；构建/门禁命令见 [docs/development.md](docs/development.md)。

## License

[Apache License 2.0](LICENSE)
