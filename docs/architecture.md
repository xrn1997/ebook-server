# 架构与形状

这份文档只回答两个问题：系统分成哪几块、为什么这么切；某条语义的主人在谁头上。它是 explanation——给读懂代码提供地图，不给代码充当副本。

不在这里的东西各有各的家：决策理由在 `docs/adr/`；构建/运行/测试在 `docs/development.md`；用户可见事实在 `README.md`；领域词汇在 `CONTEXT.md`；测试读数在 `TESTING.md` 与测试代码。

## 1. 系统形态

```
                 ┌─────────────────────────────────────────┐
  Android 客户端 │ 公开 API  0.0.0.0:<server.port>         │
                 └──────────────┬──────────────────────────┘
                                │
                                │  Go 后端（单进程）
                                │
                 ┌──────────────┴──────────────────────────┐
                 │ 后台 API 127.0.0.1:<admin.listen_port>  │
                 │ 内嵌后台前端 /admin                     │
                 └─────────────────────────────────────────┘
```

- 公开 API 与后台 API 是两个独立 Gin 引擎、两个监听地址；后台默认只监听回环地址（ADR-0010）。
- 后台前端源码在 `frontend/`，构建产物镜象进 `backend/internal/admin/web` 并由 go:embed 进二进制；运行时不另起前端服务。

## 2. 依赖方向

```
handler → service → repository → model
   ↑          ↑
 router     config
   ↑
 main.go（唯一装配点）
```

- `handler` 只做 HTTP 协议转换；校验失败走 `errcode.RespondBindError`。
- `service` 编排业务规则与事务顺序；通过 `service/ports.go` 的 consumer-defined Store 访问数据。
- `repository` 只做数据存取；gorm 类型不出 repository；查不到在 adapter 边界换成领域哨兵。
- `model` 是基础层，不依赖上层；`pkg/` 不反向依赖 handler/service/repository/router。
- `router` 只装配路由与中间件；`main.go` 只装配依赖、开监听、等退出。
- 后台页面只经 `frontend/src/api.ts` 调 `/admin/api`；`views` 不自行拼基址、不缓存第二份会话。

## 3. 唯一主人清单

| 语义 | 唯一主人 | 它不回答什么 |
| --- | --- | --- |
| 路由名、方法、鉴权挂载 | `backend/router/router.go` + `backend/router/router_test.go` | 业务判据 |
| 响应信封、业务码、端点级覆盖 | `backend/pkg/errcode` + `model.SuccessCode` + ADR-0001/0006 | 错误文案的 UI 呈现 |
| 配置字段、默认值、环境变量覆盖、校验规则 | `backend/config/config.go` 的 `mapstructure` 标签 + `backend/config/schema.go` + ADR-0016 | 运行时环境探测 |
| 请求作用域取消与超时 | `backend/service/ports.go` 的 ctx 签名 + `backend/repository` 的 `WithContext` + ADR-0022 | 外部没有取消路径的 helper（SMTP/文件删除） |
| SQLite 连接参数（WAL/busy_timeout/foreign_keys） | `backend/pkg/database` 的 `dsnOf` + `database_test.go` | 业务查询 |
| schema 版本与迁移 | `backend/migrations` + `backend/pkg/database` 的 `Migrate` + ADR-0023 | 测试库的 AutoMigrate |
| 请求关联 id | `backend/middleware/requestid.go` + Logger | 业务身份 |
| 登录主标识、账号根标识 | `backend/model/user.go` + ADR-0002/0004 | 展示用名 |
| 账号资料与头像生命周期 | `backend/service/profile.go` + `backend/pkg/upload` + ADR-0011/0017 | 前端裁剪/展示 |
| 凭据、失败计数、锁定、改密撤销会话 | `backend/service/credentials.go` + ADR-0017 | 登录页文案 |
| 评论聚合键、章节/书籍级评论 | `backend/model/comment.go` + `backend/service/comment.go` + ADR-0013/0014 | 客户端如何派生键 |
| 评论删除范围 | `backend/model/comment.go` 的 `CommentScope` + `backend/service/comment.go` + ADR-0015 | 权限等级 |
| 操作审计的接入边界与记录规则 | `backend/router/router.go`（挂载）+ `backend/service/log.go`（记录规则）+ ADR-0018 | 日志投递目的地 |
| 后台页面与后台 API 调用 | `frontend/src/api.ts` + `frontend/src/useAdminList.ts` | 业务判据 |

## 4. 不变量

- 业务端点 HTTP 恒为 200，成败由 `{code,error,data}` 信封表述；请求绑定/校验失败是唯一例外出口 `errcode.RespondBindError`。
- 需要登录的路由一律挂在路由组上，不逐路 inline；新路由必须登记进路由表测试。
- 审计只挂 `/api` 组；`/admin*` 与 `/health` 不是任何人的操作，不入库。
- 两个引擎都不信任客户端代理头（`SetTrustedProxies(nil)`）；审计 IP 与后台登录限流只认连接对端地址。
- 后台使用独立 JWT；管理员凭据只来自部署配置，页面仅持有短期管理 token。
- 配置的显式空值优先于默认值；`config.Load` 拒绝空密钥、非法端口、非法运行模式/有效期与通配后台监听。
- service/repository 的请求作用域方法第一参数是 `context.Context`；handler 传 `c.Request.Context()`，repository 使用 `WithContext`。
- 生产建表与结构变更走 `backend/migrations`；新增 schema 变更必须加迁移文件，测试库的 AutoMigrate 不是生产路径。
- Logger 中间件不读取 request body；请求关联 id 由 `RequestID` 中间件写入响应头与上下文。
- 评论 `comment_key` 服务端只存不解释；空键不代表书籍级评论，而是换键前的历史行。
- 头像列与文件同生命周期：换头像删旧文件前按文件名确认没有其他账号引用；注销时尽力删除。
- 后台页面只经 `frontend/src/api.ts` 调 `/admin/api`；管理 token 存 `localStorage`，鉴权失败清 token 回登录页。

## 5. 不在这里

- 决策为什么这么定、否掉了什么：`docs/adr/`。
- 构建、运行、测试、发版、环境坑：`docs/development.md`。
- 用户怎么用、API 端点、配置样例、部署：`README.md`。
- 测试策略与读数：`TESTING.md` 与测试代码。
- 术语定义：`CONTEXT.md`。
