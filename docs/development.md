# 开发手册

本文件只回答“怎么构建、怎么运行、怎么验、怎么发版”。架构与归属见 `docs/architecture.md`；用户用法见 `README.md`；测试策略与读数见 `TESTING.md`；决策理由见 `docs/adr/`。

## 环境要求

- Go 1.23 或更高版本（`backend/go.mod` 的 `go` 指令即最低版本）。
- Node.js 与 npm（构建/测试 `frontend/` 时需要）。
- GNU make（仓库不自带；Windows 上装 MinGW 的 `mingw32-make` 也可，但命令名需自行对应）。
- Windows 上使用 Git Bash 运行 Makefile 与本文命令。
- Windows 构建单 exe 需要 MinGW-w64 的 `windres`（WinLibs 自带；不在 PATH 时用 `WINDRES=/path/to/windres` 覆盖）。
- 静态检查使用 `go vet`（见下方提交前门禁）；仓库未引入 golangci-lint 配置。

## 构建与运行

### 后端

```bash
cd backend
go mod tidy
go build -o ../ebook-server .
cd ..
./ebook-server
```

服务必须从仓库根启动：`config.yaml`、`ebook.db`、`uploads` 都按**工作目录**解析。在 `backend/` 下执行 `go run main.go` 会读不到配置；`make run` 已封装“在 `backend/` 构建、回仓库根运行”这两步。

```bash
make run
```

Windows 上 `make build` / `make windows` 也会通过 `backend-winres` 嵌入 PE VERSIONINFO；在非
Windows 主机上交叉构建 Windows 产物同样需要 `windres`。`BuildTime` 默认取 HEAD 的提交时间
（UTC），同一 commit 重复构建不会因当前时间改变 hash；可用 `BUILD_TIME=...` 覆盖。

### 后台前端

```bash
cd frontend
npm ci                 # 按 package-lock.json 安装；已有 node_modules 时可跳过
npm run dev            # vite 开发服务器（/admin/api 与 /api-docs 已代理到后台引擎）
npm run typecheck      # vue-tsc --noEmit
npm test               # vitest run
npm run build          # 产物落 frontend/dist，由 make all/build 镜像进 embed 目录
```

`make all` 会先构建前端、把 `frontend/dist` 镜像进 `backend/internal/admin/web`，再编译 Go 二进制；`make build` 复用已有产物。`make frontend-test` 是 typecheck + vitest 的门禁。

## Makefile 快捷命令

| 命令 | 作用 |
| --- | --- |
| `make all` | 先构建后台前端，再镜像 `frontend/dist` 并构建 Go 后端 |
| `make build` | 仅构建 Go 后端（复用已有的 `frontend/dist`） |
| `make run` | `backend/` 构建到 `build/`，再从仓库根启动 |
| `make docs` | 重新生成 Swagger 文档（`backend/docs/`） |
| `make frontend-test` | 前端门禁：`vue-tsc` + `vitest run` |
| `make test` | `cd backend && go test ./...` |
| `make test-verbose` | `go test -v ./...` |
| `make test-coverage` | `go test -coverprofile=coverage.out ./... && go tool cover -html=...` |
| `make test-race` | `go test -race ./...`（提交前竞态门禁） |
| `make lint` | `go vet ./...`（当前 lint 门禁） |
| `make test-model` / `test-pkg` / `test-service` / `test-handler` | 按包运行测试 |
| `make backend-winres` | 生成 Windows exe 的 PE VERSIONINFO（仅 Windows，需要 `windres`） |
| `make fmt` | `go fmt ./...`（格式化整个 backend；提交前仍按 CONTRIBUTING 只对改动文件执行 `gofmt -w`） |
| `make deps` | `go mod tidy` |
| `make db-init` | 打印数据库初始化说明（生产建表走启动时的 `database.Migrate`） |

测试目标统一带并发放量上限（`-p`/`-parallel`，默认 4）：race/覆盖率插桩的测试二进制每个实占内存可达数百 MB，无上限时多核机器会把内存与临时目录写盘一起冲顶。放量用 `TEST_JOBS=8 make test-race`。

## 发版

发版在本机完成，不依赖 CI：

1. 跑测试：`make test`、`make frontend-test`。
2. 构建带版本号的产物：`VERSION=vX.Y.Z make all`（产物 `build/ebook-server`，Windows 为 `build/ebook-server.exe`）；
   需要交叉编译时先 `make frontend-build`，再 `make linux` 或 `make windows`（两者复用
   `frontend/dist` 并内嵌进二进制）。
3. 仓库不提供安装包与发版校验脚本；需要签名、校验和或 GitHub Releases 时自行处理。

## 测试与门禁

### 后端

```bash
cd backend
go test ./...
go test -v ./...
go test -count=1 -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
go test -v -run 'TestAuthService_Login' ./service/...
go test -v ./service/...        # 指定包
```

文档与分层门禁已包含在 `go test ./...`：`backend/internal/docscheck` 检查根 AGENTS 预算、路径/链接/ADR 元数据；`backend/internal/archtest` 检查 Go 包依赖方向。

提交前额外跑：

```bash
cd backend
go vet ./...
go test -race ./...
```

`go vet` 是当前的 lint 门禁；`go test -race` 覆盖并发数据竞争。`make lint` 与 `make test-race` 是快捷入口（Windows 下 `test-race` 会把 `GOTMPDIR` 指到工作区，避免临时目录执行限制）。

覆盖率按**逐包**看，不是把子树合并成一个数字；各包门槛与当前口径只写在 `TESTING.md`，这里不复制第二份。

`service`/`handler` 的测试夹具是包级变量，靠 `go test` 同包默认串行才安全；不要给这两包用例加 `t.Parallel()`。

### 前端

```bash
make frontend-test
# 或 cd frontend && npm run typecheck && npm test
```

`frontend/` 用 TypeScript + Vitest；视图测试在文件首行标 `// @vitest-environment happy-dom`，
其余跑 node 环境。改 `frontend/` 时 typecheck 与 vitest 都要过。详细测试约定见 `TESTING.md`。

### 手工烟测（真实后端）

前端用例只在 `fetch` 边界打桩，内嵌后台与真实 `/admin/api` 不在单测覆盖范围内。改后台界面或
后台接口后，按下面顺序过一遍：

1. `make all`，从仓库根启动 `./build/ebook-server`（Windows 为 `build/ebook-server.exe`）。
2. 打开 `http://127.0.0.1:9091/admin`，用 `config.yaml` / `.env` 的管理员账号登录。
3. 依次打开概览、用户（搜索 + 详情）、评论（搜索 + 两步删除）、日志（筛选 + 展开详情）、
   API 文档页；确认分页、筛选与删除都正常，页面无报错。后台自身流量不入审计，
   日志页显示的是客户端 API 请求。
4. 退出登录后直接访问 `/admin/#/users`，应回到登录页；公开端口的 `/api-docs`
   默认不可见，后台端口的「API 文档」页应能正常加载。

## API 文档生成

```bash
make docs           # 改 Swagger 注解后重新生成 backend/docs/
```

- `backend/config/config.go` 的 `Config` 结构体（`mapstructure` 标签）定义字段；`backend/config/schema.go` 是默认值、环境变量覆盖与校验规则的单一来源。
- 改接口注解（`@Router`/`@Param` 等）后跑 `make docs`，提交重新生成的 `backend/docs/`。

## 数据库迁移

- 新库：启动时 `backend/pkg/database` 的 `Migrate` 按文件名执行 `backend/migrations/*.sql`，基线是 `0001_init.sql`。
- 旧库（已有业务表、但 `schema_migrations` 为空）：先对当前 models 跑一次 AutoMigrate 收养，再把**当前二进制内嵌的全部版本**标记为已应用。AutoMigrate 已把库带到最新 schema，若只标记 `0001_init`，未来迁移会在下一次启动时重放并因列已存在而失败。
- 收养不会为存量库重放当前版本中的数据迁移；新增数据迁移前必须重新设计收养路径（ADR-0023）。
- 新增 schema 变更：在 `backend/migrations/` 加 `NNNN_slug.sql`（按 SQLite 方言书写，见 ADR-0023），不要只改 model。`backend/pkg/database/migrate_test.go` 会把迁移库与 AutoMigrate 库的列（含默认值）与索引（名称、唯一性、列顺序）对照，漂移即失败。
- 测试库仍由 `pkg/testdb` 的 AutoMigrate 建表；它只服务测试，不是生产迁移路径。

## 部署

```bash
make all
./build/ebook-server
```

必须在放有 `config.yaml` 的目录启动：工作目录决定配置、数据库、uploads 与日志的落点。后台默认只监听 `127.0.0.1:9091`；远程管理走 SSH 隧道/VPN，不要直接开放后台端口。用 systemd 或其它进程管理器托管时，把工作目录设为部署目录即可。

## 环境坑

- **工作目录**：`config.Load("config.yaml")` 与数据库/uploads 路径都相对工作目录；在 `backend/` 下跑二进制会读不到根目录的配置。
- **前端开发服务器**：`npm run dev` 把 `/admin/api` 与 `/api-docs` 代理到 `http://127.0.0.1:9091`（后台默认监听地址）；改了 `admin.listen_port` 时用 `ADMIN_API_ORIGIN=http://127.0.0.1:<port>` 覆盖。
- **构建产物**：`build/`、`frontend/dist/` 与 `backend/internal/admin/web/` 的镜像产物不入库；`make clean` 会清掉前端产物与 embed 镜像。
- **Windows 构建**：`make windows` 需要 MinGW-w64 的 `windres`；非 Windows 主机交叉构建同样需要。
- **SMTP 未配置**：非 release 模式会退回写日志，便于联调；release 模式仍装配 SMTP adapter，发送时显式报错，避免验证码漏进日志。
- **配置空值**：配置文件里的显式空值优先于默认值；`jwt.secret`、`admin.jwt_secret`、`upload.dir` 为空一律拒绝启动。真实密钥放 `.env`，不要写进 `config.yaml`。
- **后台可达性**：默认只听回环地址；需要局域网访问时改 `admin.listen_addr`，不要改成通配地址。

## 当前已知问题

- **验证码存储是内存 map**：单进程可用；多实例部署需迁移到 Redis 或共享存储。
- **refresh token 过期记录没有定时清理**：登出/改密会作废该账号全部 refresh token，过期记录交给数据库，暂不设清理任务。
- **SQLite 写入串行化**：连接使用 WAL + `busy_timeout(5000)` + `foreign_keys(1)`；高并发场景仍需评估迁移到 MySQL/PostgreSQL。
- **数据库迁移**：新库执行 `backend/migrations` 的 SQL；旧库启动时先 AutoMigrate 收养，并把当前二进制内嵌的全部版本标记为已应用。新增列/索引必须加迁移文件；schema 对照测试会挡下迁移与 model 的漂移。
- **access token 无法主动作废**：改密/重置/注销只删除 refresh token；无状态 access token 在剩余有效期内仍可调通 API。彻底解决需要 token 黑名单。
- **账号注销不包事务**：依次清头像、匿名化账号、删 refresh token；中途失败可重新登录重试（最后一步失败留下的令牌也续不了期，只会自行过期），不会锁死或丢数据。这是有意的取舍，不是遗漏。
- **改密与并发刷新有竞态窗口**：改密先删 refresh token、再写新哈希；`Refresh` 若在删令牌前读到旧记录、在改库后写入新令牌，这枚新令牌会存活。取舍是 fail-closed（宁可要求重新登录，也不留下「新密码生效、旧令牌仍可用」的常态）；彻底解决需要 token 版本号或改密时间戳。注销的同型竞态不在此列：漏网令牌在续期前会被匿名化状态检查拒绝（`service.AuthService.Refresh`），「注销后无限续期的会话」不成立。
- **头像文件清理的并发窗口**：换头像/注销先写库、再按引用计数删旧文件；两笔并发更新把旧 URL 写回时，仍可能让账号指向已删除文件。彻底解决需要文件级引用计数或延迟回收；当前按单机单进程、低概率取舍，不引入跨存储事务。
- **debug/test 模式仍使用内置默认密钥/口令**：`release` 模式会拒绝三份内置默认值；上线前核对 `server.mode`。

## 添加新接口的标准流程

1. `model/` — 定义数据模型与请求/响应结构体。
2. `service/ports.go` — 如需要新的数据访问能力，先在对应 Store 接口上扩方法（接口由消费方定义）。
3. `repository/` — 实现数据访问；单行查询用 `translateNotFound` 收口到本聚合的领域哨兵，不把 gorm 错误递给上层。
4. `service/` — 实现业务逻辑；跨多个写步骤时在注释里写清失败顺序与自愈语义。
5. `handler/` — 实现 HTTP 处理器：参数经 `pkg/httpx` 取，响应经 `pkg/errcode` 回；绑定/校验失败用 `errcode.RespondBindError`；当前登录身份用 `handler.CurrentUser(c)`。
6. `router/router.go` — 把依赖装配好，路由挂进 `Public` 或 `AdminConsole`；需要登录的路由挂到路由组上。
7. 编写/更新测试；路由挂载本身由 `router/router_test.go` 覆盖，漏挂鉴权会在那里失败。

依赖方向与唯一主人清单见 `docs/architecture.md`。

## 配置

配置字段定义在 `backend/config/config.go` 的 `Config` 结构体（`mapstructure` 标签）；默认值、环境变量覆盖与校验规则的唯一来源是 `backend/config/schema.go`。用户可见的配置样例在 `README.md`；本文件不抄完整 YAML。

- `.env` 覆盖 `SMTP_PASSWORD`、`JWT_SECRET`、`ADMIN_JWT_SECRET`、`ADMIN_PASSWORD` 等敏感项；`.env` 不入库。
- `config.Load` 在启动时执行校验：空密钥、非法端口、非法运行模式/有效期与通配后台监听直接拒绝。修改字段或规则时同步更新 `backend/config` 的测试。
