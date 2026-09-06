import { spawn, type ChildProcess } from 'node:child_process'
import http from 'node:http'
import { StringDecoder } from 'node:string_decoder'
import type { ServiceErrorReason, ServiceStatus } from '../shared/types'

/** SidecarManager 构造选项 */
export interface SidecarOptions {
  /** Go 二进制的绝对路径 */
  binaryPath: string
  /** 子进程工作目录（config.yaml/.env/ebook.db 所在） */
  workDir: string
  /**
   * Go 公开 API 端口（健康检查用），来自受管配置的 server.port。
   * 用函数而非数值：改配置后重启即生效，避免重启仍拨旧端口。
   */
  port: () => number
  /** 健康检查拨号主机，默认回环（后端绑 0.0.0.0，回环必可达） */
  healthHost?: string
  /** 运行平台，默认 process.platform（Windows 的停止语义不同，需可注入才能测） */
  platform?: string
  /** 状态变化回调 */
  onStatusChange: (status: ServiceStatus) => void
  /** 日志输出回调 */
  onLog: (line: string) => void
  /** 失败原因回调（设计文档要求区分「端口被占用」等具体原因） */
  onError?: (reason: ServiceErrorReason, message: string) => void
}

/**
 * 后端 listen 失败的特征串。
 * 不能只匹配英文：Windows 的 socket 错误由系统按本地语言输出（中文系统上是
 * 「通常每个套接字地址(协议/网络地址/端口)只允许使用一次」）。因此同时匹配
 * Go net 包稳定打印的 "listen tcp/udp" 前缀——bind 失败无论何种原因都意味着
 * 端口不可用，对界面而言都是同一件事。
 */
const PORT_IN_USE_PATTERN = /listen tcp |listen udp |address already in use|only one usage of each socket address|只允许使用一次|EADDRINUSE|Can't assign requested address/i

/**
 * 写进子进程 stdin 的退出指令，backend/main.go 的 waitForShutdown 认这个字面量。
 * 两侧是两个语言、必须人工保持一致，改动任何一边都要同步另一边。
 */
const SHUTDOWN_COMMAND = 'shutdown'

/**
 * Go sidecar 子进程管理器。
 * 负责 spawn/kill/restart Go 后端二进制，并通过健康检查确认就绪。
 */
export class SidecarManager {
  private process: ChildProcess | null = null
  private status: ServiceStatus = 'stopped'
  private healthTimer: ReturnType<typeof setInterval> | null = null
  private startTimeout: ReturnType<typeof setTimeout> | null = null
  private forceKillTimer: ReturnType<typeof setTimeout> | null = null
  private restartTimer: ReturnType<typeof setTimeout> | null = null
  private restartCount = 0
  private readonly maxRestarts = 3
  private intentionalStop = false
  private portConflictSeen = false
  private runningSince: number | null = null
  /** 本次启动所用端口快照（端口是每次启动求值的，健康检查与文案必须引用同一个值） */
  private activePort = 0
  /** 行解码器：多字节 UTF-8 字符跨 chunk 到达时不会产生替换符 */
  private lineDecoders: { stdout: StringDecoder; stderr: StringDecoder } = {
    stdout: new StringDecoder('utf8'),
    stderr: new StringDecoder('utf8'),
  }
  /** 未凑齐整行的半行缓存，等下一次 data 或进程退出时输出 */
  private pendingLines: { stdout: string; stderr: string } = { stdout: '', stderr: '' }

  constructor(private opts: SidecarOptions) {}

  /** 获取当前状态 */
  getStatus(): ServiceStatus {
    return this.status
  }

  /**
   * 服务已持续运行的毫秒数（非 running 状态返回 0）。
   * 概览页的「运行时间」用它，避免让 renderer 自己计时造成漂移。
   */
  getUptimeMs(): number {
    return this.runningSince === null ? 0 : Math.max(0, Date.now() - this.runningSince)
  }

  /** 更新状态并通知 */
  private setStatus(s: ServiceStatus): void {
    this.status = s
    if (s === 'running' && this.runningSince === null) {
      this.runningSince = Date.now()
    }
    if (s !== 'running') {
      this.runningSince = null
    }
    this.opts.onStatusChange(s)
  }

  private get platform(): string {
    return this.opts.platform ?? process.platform
  }

  private get healthHost(): string {
    return this.opts.healthHost ?? '127.0.0.1'
  }

  /** 启动 Go 子进程 */
  start(): void {
    // stopping 也挡：宽限期内点「启动」会去抢旧进程还没释放的端口，
    // 得到一次假的「端口被占用」报错；等 stop 流程落回 stopped 后再启动。
    if (this.status === 'starting' || this.status === 'running' || this.status === 'stopping') return
    this.cancelRestart()
    this.intentionalStop = false
    this.portConflictSeen = false
    this.activePort = this.opts.port()
    this.lineDecoders = { stdout: new StringDecoder('utf8'), stderr: new StringDecoder('utf8') }
    this.pendingLines = { stdout: '', stderr: '' }
    this.setStatus('starting')
    this.opts.onLog(`[sidecar] Starting ${this.opts.binaryPath}`)

    try {
      this.process = spawn(this.opts.binaryPath, [], {
        cwd: this.opts.workDir,
        // stdin 用 pipe：stop 时要写优雅退出指令（backend 的 waitForShutdown 监听）
        stdio: ['pipe', 'pipe', 'pipe'],
        env: { ...process.env },
      })
    } catch (error) {
      // 二进制缺失/不可执行时 spawn 同步抛错：给出可操作的提示而不是崩在后台
      const message = `无法启动后端进程：${String(error)}`
      this.opts.onLog(`[sidecar] ${message}`)
      this.setStatus('error')
      this.opts.onError?.('spawn-failed', message)
      return
    }

    this.process.stdout?.on('data', (chunk: Buffer) => {
      this.emitLines('go:stdout', 'stdout', chunk)
    })

    this.process.stderr?.on('data', (chunk: Buffer) => {
      this.emitLines('go:stderr', 'stderr', chunk)
    })

    // 子进程死亡后 stdin 可能还有在途写入（EPIPE），吞掉即可：stop 流程有强杀兜底
    this.process.stdin?.on('error', () => {})

    // 二进制不存在时 spawn 不抛错，而是异步 emit 'error'（ENOENT）
    this.process.on('error', (error: Error) => {
      const message = `后端进程错误：${error.message}`
      this.opts.onLog(`[sidecar] ${message}`)
      this.intentionalStop = true
      this.cleanup()
      this.setStatus('error')
      this.opts.onError?.('spawn-failed', message)
    })

    this.process.on('exit', (code, signal) => {
      this.opts.onLog(`[sidecar] Process exited (code=${code}, signal=${signal})`)
      this.cleanup()
      // 端口冲突已被定性为 error：进程随之退出，不要把状态改回 stopped 掩盖原因
      if (this.portConflictSeen) return
      // 退出时兜底吐出残留半行：end() 的返回值没有换行符，不能走 emitLines（会再次被缓存而静默丢失）
      for (const [stream, prefix] of [
        ['stdout', 'go:stdout'],
        ['stderr', 'go:stderr'],
      ] as const) {
        const residual = (this.pendingLines[stream] + this.lineDecoders[stream].end()).trim()
        if (residual) this.opts.onLog(`[${prefix}] ${residual}`)
      }
      this.pendingLines = { stdout: '', stderr: '' }
      if (!this.intentionalStop) {
        this.handleCrash()
      } else {
        this.setStatus('stopped')
      }
    })

    this.startHealthCheck()
    this.startTimeout = setTimeout(() => {
      if (this.status === 'starting') {
        this.opts.onLog('[sidecar] Health check timeout (10s), marking as error')
        this.intentionalStop = true
        const proc = this.process
        this.cleanup()
        this.setStatus('error')
        this.opts.onError?.('startup-timeout', `后端在 10 秒内未通过健康检查（http://${this.healthHost}:${this.activePort}/health）`)
        if (proc) {
          this.terminate(proc)
        }
      }
    }, 10_000)
  }

  /** 按行转发子进程输出：StringDecoder 处理跨 chunk 的多字节字符，半行先缓存等凑齐 */
  private emitLines(prefix: string, stream: 'stdout' | 'stderr', chunk: Buffer): void {
    const text = this.lineDecoders[stream].write(chunk)
    const buffered = this.pendingLines[stream] + text
    const lines = buffered.split(/\r?\n/)
    this.pendingLines[stream] = lines.pop() ?? ''
    for (const line of lines.filter(Boolean)) {
      this.opts.onLog(`[${prefix}] ${line}`)
      if (!this.portConflictSeen && PORT_IN_USE_PATTERN.test(line)) {
        this.reportPortConflict()
      }
    }
  }

  /**
   * 端口被占用：立即失败，不等健康检查超时。
   * 关键点是此时任何 /health 的回应都可能来自**占用该端口的另一个进程**，
   * 继续轮询会把我们误标成 running；而这是确定性故障，不属于崩溃，
   * 故置 intentionalStop，让子进程退出后不再触发无意义的自动重启。
   */
  private reportPortConflict(): void {
    this.portConflictSeen = true
    this.intentionalStop = true
    const proc = this.process
    this.cleanup()
    this.setStatus('error')
    const message = `端口 ${this.activePort} 被占用，后端未能启动。请在配置页改用其他端口。`
    this.opts.onLog(`[sidecar] ${message}`)
    this.opts.onError?.('port-conflict', message)
    if (proc) {
      this.forceKill(proc)
    }
  }

  /** 停止 Go 子进程 */
  stop(): void {
    this.intentionalStop = true
    this.cancelRestart()
    const proc = this.process
    this.cleanup()
    if (proc) {
      this.forceKillAfterGrace(proc)
    } else {
      this.setStatus('stopped')
    }
  }

  /** 重启：停止后启动 */
  async restart(): Promise<void> {
    this.intentionalStop = true
    this.cancelRestart()
    const proc = this.process
    this.cleanup()
    if (proc) {
      this.forceKillAfterGrace(proc)
      await new Promise<void>((resolve) => {
        proc.once('exit', () => resolve())
        setTimeout(resolve, 6000)
      })
    }
    this.start()
  }

  /**
   * 请求优雅退出：先向 stdin 写退出指令，5 秒宽限期过后才强杀（设计文档）。
   *
   * 为什么走 stdin 而不是信号：Windows 上无窗口的控制台子进程收不到 SIGTERM
   * （taskkill 不带 /F 对它们直接无效），「先信号、等 5 秒、强杀」会永远耗尽宽限期；
   * 而父子进程双方都拿得到的通道只有 stdin。Go 侧 waitForShutdown 收到指令后
   * 主动 Shutdown 两个监听并关库，通常远快于 5 秒；指令写入失败（进程已死等）
   * 则只剩强杀兜底。
   */
  private forceKillAfterGrace(proc: ChildProcess): void {
    this.setStatus('stopping')
    this.requestGracefulExit(proc)
    this.forceKillTimer = setTimeout(() => {
      this.forceKillTimer = null
      this.forceKill(proc)
    }, 5000)
    proc.once('exit', () => {
      if (this.forceKillTimer) {
        clearTimeout(this.forceKillTimer)
        this.forceKillTimer = null
      }
    })
  }

  /** 向子进程 stdin 写退出指令并关闭写端；stdin 不可写时静默失败（强杀兜底） */
  private requestGracefulExit(proc: ChildProcess): void {
    try {
      proc.stdin?.write(`${SHUTDOWN_COMMAND}\n`)
      proc.stdin?.end()
    } catch {
      // 进程已死时 stdin 写入会抛错，无需处理
    }
  }

  /**
   * 尽力而为的温和终止，只用于「健康检查超时的僵死启动」兜底。
   * 注意 Windows 上 taskkill 不带 /F 对无窗口控制台程序通常无效，真正兜底的是
   * forceKill；POSIX 上它等价于 SIGTERM。
   */
  private terminate(proc: ChildProcess): void {
    if (this.platform === 'win32') {
      this.runTaskkill(proc, false)
      return
    }
    try { proc.kill('SIGTERM') } catch { /* already dead */ }
  }

  /** 强制杀死（清理定时器/端口的最后手段） */
  private forceKill(proc: ChildProcess): void {
    if (this.platform === 'win32') {
      this.runTaskkill(proc, true)
      return
    }
    try { proc.kill('SIGKILL') } catch { /* already dead */ }
  }

  private runTaskkill(proc: ChildProcess, force: boolean): void {
    if (proc.pid === undefined) return
    const args = force ? ['/F', '/T', '/PID', String(proc.pid)] : ['/PID', String(proc.pid)]
    let killer: ChildProcess
    try {
      killer = spawn('taskkill', args, { stdio: 'ignore' })
    } catch {
      // taskkill 不可用时退回 Node 自带信号，至少不留下孤儿进程
      try { proc.kill(force ? 'SIGKILL' : 'SIGTERM') } catch { /* already dead */ }
      return
    }
    killer.once('error', () => {
      // spawn 的 ENOENT 走异步 error 事件，同步 try/catch 罩不住
      try { proc.kill(force ? 'SIGKILL' : 'SIGTERM') } catch { /* already dead */ }
    })
  }

  /** 取消待执行的崩溃自动重启（用户显式停止后不得再复活进程） */
  private cancelRestart(): void {
    if (this.restartTimer) {
      clearTimeout(this.restartTimer)
      this.restartTimer = null
    }
  }

  /** 清理定时器和进程引用 */
  private cleanup(): void {
    if (this.healthTimer) {
      clearInterval(this.healthTimer)
      this.healthTimer = null
    }
    if (this.startTimeout) {
      clearTimeout(this.startTimeout)
      this.startTimeout = null
    }
    this.process = null
  }

  /** 启动周期性健康检查 */
  private startHealthCheck(): void {
    this.healthTimer = setInterval(() => {
      this.checkHealth()
    }, 1000)
  }

  /** 单次健康检查：GET /health，200 则标记为 running */
  private checkHealth(): void {
    const req = http.get(`http://${this.healthHost}:${this.activePort}/health`, (res) => {
      if (res.statusCode === 200 && this.status === 'starting') {
        this.opts.onLog('[sidecar] Health check passed, service is running')
        if (this.startTimeout) {
          clearTimeout(this.startTimeout)
          this.startTimeout = null
        }
        // Clear health check interval to prevent resource leak
        if (this.healthTimer) {
          clearInterval(this.healthTimer)
          this.healthTimer = null
        }
        this.setStatus('running')
        this.restartCount = 0
      }
    })
    req.on('error', () => {
      // 健康检查失败，继续等待
    })
    req.setTimeout(2000, () => req.destroy())
  }

  /** 进程异常退出时自动重启（最多 maxRestarts 次，递增延迟） */
  private handleCrash(): void {
    // 走到这里必然不是端口冲突：exit handler 对 portConflictSeen 已提前 return
    const reason: ServiceErrorReason = 'crashed'
    const message = '后端进程意外退出。'

    if (this.restartCount >= this.maxRestarts) {
      this.opts.onLog('[sidecar] Max restart attempts reached, marking as error')
      this.setStatus('error')
      this.opts.onError?.(reason, `${message}已连续 ${this.maxRestarts} 次自动重启失败，不再重试。`)
      return
    }

    this.restartCount++
    const delay = this.restartCount * 2000
    this.opts.onLog(`[sidecar] Auto-restart attempt ${this.restartCount}/${this.maxRestarts} in ${delay}ms`)
    this.setStatus('error')
    this.opts.onError?.(reason, `${message}将在 ${delay / 1000} 秒后自动重启（第 ${this.restartCount}/${this.maxRestarts} 次）。`)
    this.restartTimer = setTimeout(() => {
      this.restartTimer = null
      this.process = null
      this.start()
    }, delay)
  }
}
