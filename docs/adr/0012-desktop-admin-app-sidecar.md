# 桌面管理应用：Go 后端以 sidecar 复用，配置与凭据由主进程托管

为「零命令行管理自己的 ebook-server」这一需求增加 `desktop/`（Electron + Vue 3）。
设计文档见 `docs/superpowers/specs/2026-09-05-desktop-app-design.md`。本 ADR 记录其中
三条会长期约束后续改动的决策：进程拓扑、配置所有权、后台鉴权的取得方式。

## 现状

后台管理能力此前只有一条路径：浏览器访问 `127.0.0.1:9091/admin`（ADR-0009/0010）。
它要求使用者自己准备 Go 运行时、自己编辑 `config.yaml`、自己把敏感项写进 `.env`、
自己起进程并盯日志。对个人部署而言这一步成本过高，而桌面应用想复用的恰恰是
**同一套后端与同一套后台 API**，不是第二份实现。

## 决策

### 1. 后端作为 sidecar 子进程复用，桌面端零业务逻辑

Electron 主进程 spawn 已编译的 Go 二进制，工作目录设为用户数据目录，轮询
`/health` 判定就绪；停止时请求优雅退出、5 秒宽限期后强杀。桌面端**不**实现任何
业务读写，所有数据仍由 Go 后端提供：

* Go 代码零改动即可被桌面应用带动（含既有 `/admin/api/*`）；
  （后续修正：「零改动」只对**读**成立。桌面应用真正可用需要后端补优雅关闭
  与后台管理面的搜索/详情/删除/筛选，见 [ADR-0013](0013-comment-key-migration-and-admin-api.md)）；
* 桌面应用崩溃或被替换都不影响服务端数据；
* 同一份二进制既能 `go run` 也能被打包进安装包（`extraResources`）。

代价是要处理子进程生命周期（僵尸进程、端口未释放、崩溃自动重启），这部分逻辑集中在
`desktop/src/main/sidecar.ts`。

### 2. 用户数据目录里的 config.yaml + .env 由主进程独占管理

`config.yaml`（非敏感）与 `.env`（`JWT_SECRET`/`SMTP_PASSWORD`/`ADMIN_PASSWORD`/
`ADMIN_JWT_SECRET`）都存在 Electron 的 `userData/ebook-server/` 下，只有主进程读写，
renderer 一律经 IPC 取用（后续修正：目录不再多套一层 `ebook-server` 子目录——
主进程 `app.setPath('userData', appData/ebook-server)` 固定路径，否则 dev 用
package.json 的 name、打包用 productName，两边落到不同目录，配置互不相通）：

* 保存前按已知结构收敛（`sanitizeConfig`）：renderer 传来的未知键一律丢弃，
  一份畸形配置不能污染后端启动文件；
* 密钥只进 `.env`，因此磁盘上的 `config.yaml` 不含任何明文凭据；
* **主进程是端点的唯一来源**：`server.port`、`admin.listen_port`、`database.path` 都
  由它解析后交给界面与健康检查。写死 9090/9091 会让用户改完端口后功能静默失效。

后端侧的配套约束：`viper.Unmarshal` 只遍历它已知的键，所以 `.env` 要能覆盖
`smtp.password`，该键必须在 `config.go` 里注册默认值（否则 GUI 配的 SMTP 密码永远传不到
发信逻辑）。

### 3. 管理员凭据不出主进程：由主进程自动登录换取后台 JWT

用户/评论/日志三个视图都要访问 `/admin/api/*`，而这些端点全部在 ADR-0010 的独立后台
鉴权之后。桌面应用不再摆一个登录表单让用户重复输入自己刚在配置页填过的密码，而是由
主进程用同一份受管配置里的 `admin.username`/`admin.password` 调 `POST /admin/api/login`，
把换来的 JWT 交给 renderer（renderer 只拿得到可过期的令牌，拿不到密码）：

* 凭据与端点是**一体两面**：二者都由主进程持有，故「自动登录必然成功」的前提是
  后端启动时读的正是这份 `.env`；
* token 生命周期跟随子进程：进入 `running` 时换取，停止/报错时作废
  （已实现：`stopped` 与 `error` 都会 invalidate）；
* 后台 JWT 60 分钟过期（ADR-0010），故 renderer 收到 `A0403/A0240/A0230` 时向主进程
  强制重取一次并重试；换取请求在两侧都做合流，避免并发打爆 5 次/分钟的登录限流（`A0241`）。

### 4. 后台监听地址不接受通配值

GUI 暴露 `admin.listen_addr` 供局域网管理（ADR-0010 文档给出的用法），但保存时**拒绝**
`0.0.0.0`/`::`/`*`：通配监听会把后台端口整个暴露给公网，而文档推荐的局域网用法本来就
是填具体内网 IP。作为同一决策的另一半，连接时会把通配地址换算成 `127.0.0.1`
（通配地址是绑定语义，不是可拨号目标）。

## 被拒方案

* **把后台管理逻辑在桌面端重写一遍**：等于第二套真相，与「复用现有 Vue 视图」矛盾。
* **内嵌 Go 代码进 Electron（同进程 go-native 插件）**：需要 CGO 与跨平台原生构建，
  换来的只是省一个子进程；而 sidecar 崩了桌面端还能显示日志，隔离反而是想要的性质。
* **移植一个手动登录页**：凭据明明由桌面应用自己托管，让用户再输入一遍是重复劳动，
  且会把密码送进 renderer 的 localStorage。
* **让 renderer 直接读 `config.yaml`/`.env`**：等于把文件解析与密钥暴露到不可信侧。
* **GUI 允许 `0.0.0.0` 但加一个二次确认弹窗**：确认框挡不住一个不知道自己点了什么的人，
  而 ADR-0010 的隔离是网络层的物理性质，禁用通配并不损失任何被文档支持的能力。

## 下游影响

* `desktop/` 成为 monorepo 第三个顶层成员（ADR-0009 只描述了 `backend/` + `frontend/`），
  有独立 npm 工具链、独立测试与打包（`make desktop-*`）。
* 端口/凭据改动必须重启才生效，因此界面要如实显示「配置已保存，需重启服务」，
  不能假装已生效。
* 打包产物里的 sidecar 路径由 `electron-builder.yml` 的 `extraResources` 与
  `Makefile: desktop-build-backend` 共同决定，两处必须一致（开发模式与打包模式都按
  `resources/backend/` 查找）。
* 管理员密码留空时后台接口不可用，此时桌面应用显示「管理员账号或密码未设置」，
  而不是让三个视图静默空白。

## 验收

* `npm run typecheck && npm test`（`desktop/` 下）全绿；`go test ./...` 全绿。
  桌面测试用 `make desktop-test` 跑：它先编译 sidecar，保证真后端集成用例
  不会因二进制缺失而整段自我跳过。
* 概览页能看到端口、数据库文件大小、运行时间与「需重启」提示（设计文档的运行监控要求）。
* 改端口并重启后健康检查拨新端口；`admin.listen_addr` 填 `0.0.0.0`（及 `[::]` 等通配写法）
  无法保存。
* 用户/评论/日志三个视图在只配置过管理员密码、从未手动登录的情况下能载入数据，
  且具备设计文档要求的搜索/详情/删除/筛选/分页（ADR-0013）。
* `.env` 里写 `SMTP_PASSWORD` 后，`config_test.go` 证明后端读到的 `smtp.password` 非空。
* 停止服务走真优雅退出（后端 `signal.Notify`/`Shutdown` + stdin `shutdown` 指令），
  集成测试断言停止耗时远小于 5 秒宽限期。
