# sidecar 的停止/重启以 Promise 表达

Status: superseded by 0024

`SidecarManager.stop()` 曾经返回 `void`，但停止是一个持续 5 秒的动作（写 `shutdown` 到
stdin → 等进程自行退出 → 宽限期到点强杀）。调用方只能自己猜它什么时候落定：

* `before-quit` 每 100ms 轮询 `getStatus() === 'stopping'`，最多等 6 秒。
* `restart()` 先调 `stop()`，再注册一次 `onExit` 并用 `setTimeout(resolve, 6000)` 兜底。
* 崩溃退避期间状态是 `error`——进程马上会回来，界面却报「异常」，后台会话也过早把它
  当成终态。

三处等待各自实现，语义靠时序巧合对齐。

## 决策

### 1. `stop(): Promise<StopOutcome>`

`StopOutcome` 只有两种：`'exited'`（宽限期内自行退出）与 `'killed'`（宽限期到点、已发出
强杀指令）。Promise 在任一条件先发生时落定一次；重复调用 `stop()` 复用同一个 Promise，
不会写两次 stdin、装两套定时器。没有子进程时立即 resolve `'exited'`；状态只从过渡态
落回 `stopped`，已经是 `error` 时保留 error——那是「为什么没起来」的唯一记录，一次
多余的停止不该把它抹掉。

「没有子进程」必须是事实，而不是引用的丢失：`cleanup()` 会先置空 `child`，而启动超时与
端口冲突两条路径正是在那之后才去终止进程——Windows 上不带 `/F` 的 taskkill 对无窗口控制台
子进程通常无效，于是进程还活着却已经没人记得它，`stop()` 会当场回一个 `'exited'`，
`before-quit` 也就带着一个仍在服务（且绑在 `0.0.0.0`）的后端退出。两条路径现在把进程
登记在 `stoppingChild` 上，`stop()`/`restart()` 仍能找到它并走完停止。相应地，退出与错误事件都只认「现在还归我管」
的那个进程：被新一次启动取代的旧进程迟到退出或迟到报错时，不得清掉新进程的启动超时与
健康检查间隔，也不得被算成一次崩溃。

进程退出时的状态落定还区分两种 intentionalStop：显式停止只从 `stopping` 落回 `stopped`；
启动超时同样会终止进程，但那已经是终态失败，退出事件不得把 `error` 覆盖成 `stopped`。
端口冲突的退出事件原本会提前返回以保住 `error`，现在还要处理「用户在冲突后显式停止」的
情形——只有当前状态是 `stopping` 才落回 `stopped`，否则会永远卡在 `stopping`。

端口冲突的识别只看**启动阶段的 stderr**：Go 标准库把 listen 失败写到 stderr，而 Gin 的
请求日志写 stdout、path 完全由匿名客户端控制。若运行期也扫 stdout，一个含
`address already in use` 的请求路径就能让桌面端杀掉自己健康的后端。从 `error` 再次
`start()` 前必须先强杀 `stoppingChild`：Windows 上温和终止常常无效，直接丢引用会留下
一个 `stop()`/退出流程都找不到的孤儿进程。

### 2. `restart()` 等旧进程真正退出后再启动

`await stop()` 之后还要再等一次 exit：宽限期到点强杀时 Promise 返回 `'killed'`，而进程
可能还差一瞬间才退出；此时拉起新进程会去抢尚未释放的端口，得到一次假的「端口被占用」。
这次等待有 6 秒兜底（与旧实现的等待上限一致）；超时仍未退出时落到 `error` 并报
`stop-timeout`，不再调用会被 `stopping` 守卫静默挡下的 `start()`。等待期间若用户又点了
停止，`stopSeq` 会让 restart 直接放弃 `start()`，不把用户刚要停下的服务复活。

### 3. 每次拉起进程前对齐受管配置

`SidecarOptions.prepareLaunch` 在每次真正 spawn 前调用（包括崩溃退避后的自动重启）。
主入口用它重新执行 `ConfigService.loadForRun`，把 `runtimeConfig` 与磁盘配置对齐，再让
`endpoints()` 求值；返回 false 时 sidecar 落到 `error`、不拉起进程。后端进程每次启动都会
重读 `config.yaml`，若崩溃重启沿用旧快照，健康检查会拨旧端口，把已经在新端口上健康的
进程误判成启动超时后强杀。手动启动/重启仍由 `applyServiceConfig` 先做一次同样的校验，
`prepareLaunch` 是最后一道、也是自动重启唯一的一道。

### 4. 状态词表增加 `restarting`

* 崩溃进入退避窗口时状态是 `restarting`，不是 `error`；重试用尽才是终态 `error`。
* `start()` 在 `restarting` 期间是 no-op（手动启动不能与退避计时器各拉一个进程）；
  退避到点由内部的 `launch()` 执行，绕过「谁可以发起启动」的守卫。
* 后台会话在 `stopped` / `restarting` / `error` 时都作废 token；`running` 时预热。
* 界面显示「重启中」，启动/重启按钮按 `starting` 的规则禁用，停止可用（取消退避）。

### 5. 退出流程不再轮询

`before-quit` 在停止落定前对**每一次**触发都 `preventDefault`，等 `stop()` 的 Promise
落定后才允许真正 `app.quit()`；第二次触发（托盘退出、window-all-closed）不得绕过等待。
停止落定后才注销 IPC handler：停止最长 5 秒，期间 renderer 仍在轮询运行快照，提前摘
handler 会让这些轮询变成未处理的 Promise 拒绝。renderer 启动时还会从主进程的 `lastError`
快照回填启动失败；「忽略」只在本地记账，状态转 `running` 时清空，轮询不会把同一条失败
一秒后填回。不再有 100ms 轮询与 6 秒 deadline。`SHUTDOWN_COMMAND = 'shutdown'` 仍是跨语言字面量：
后端曾有的 `ShutdownCommand` 与 `desktop/src/main/sidecar.ts` 各一份，
改动任一侧必须同步另一侧。

## 后果

* 停止/重启的等待语义只在一个模块里定义；IPC 的 `STOP_SERVICE` 可以等待落定再回包。
* `killed` 只表示「已发出强杀」，不保证进程已退出；需要「确已落定」的调用方
  （如 restart）必须像本模块一样再等 exit；等不到时 restart 落 `stop-timeout`，不会静默停在 `stopping`。
* 自动重启与手动启动都会先经过 `prepareLaunch`：后端每次启动重读的配置与桌面端健康检查 /
  后台会话使用的快照保持同一份；配置非法时不 spawn 一个注定退出、还会被误判为超时的进程。
* 崩溃退避不再是可见的「异常」，界面不会在每次自动重启时闪一次红色错误；
  `onError` 回调仍会上报「将在 N 秒后自动重启」。
* Windows 上的停止语义（stdin 指令 + `taskkill /F /T` 兜底）不变，只是结果可等待。

测试：`desktop/src/main/__tests__/sidecar.test.ts`（stop 的两种结果、重复调用、
退避期间 `restarting`、restart 等旧进程退出、旧进程宽限内未退出时 restart 落 `stop-timeout`、
每次拉起前执行 `prepareLaunch`、状态序列、被放弃的进程仍归 stop 管、
旧进程迟到退出/迟到报错不改写新一次启动、启动超时的 `error` 不被退出事件覆盖、
端口冲突后显式停止不会卡在 `stopping`）、`ipc.test.ts`
（`STOP_SERVICE` 等待 stop）、`quit.test.ts`（停止落定前每次触发都 preventDefault、
停止期间保留 IPC、等 stop 落定才注销并 quit、stop 被拒也能退出）、
`service-store.test.ts`（lastError 回填、忽略后不被轮询填回、转 running 清空）、
`admin-auth.test.ts`（`restarting` 作废 token）、
`backend-integration.test.ts`（真二进制优雅退出远快于 5 秒）。

锚点：`backend/main.go` 的 `shutdown`；ADR-0024。
