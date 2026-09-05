import { spawn, type ChildProcess } from 'node:child_process'
import http from 'node:http'
import path from 'node:path'

/** Sidecar 子进程状态 */
export type SidecarStatus = 'stopped' | 'starting' | 'running' | 'stopping' | 'error'

/** SidecarManager 构造选项 */
export interface SidecarOptions {
  /** Go 二进制的绝对路径 */
  binaryPath: string
  /** 子进程工作目录 */
  workDir: string
  /** Go 服务监听端口（用于健康检查） */
  port: number
  /** 状态变化回调 */
  onStatusChange: (status: SidecarStatus) => void
  /** 日志输出回调 */
  onLog: (line: string) => void
}

/**
 * Go sidecar 子进程管理器。
 * 负责 spawn/kill/restart Go 后端二进制，并通过健康检查确认就绪。
 */
export class SidecarManager {
  private process: ChildProcess | null = null
  private status: SidecarStatus = 'stopped'
  private healthTimer: ReturnType<typeof setInterval> | null = null
  private startTimeout: ReturnType<typeof setTimeout> | null = null
  private restartCount = 0
  private readonly maxRestarts = 3
  private intentionalStop = false

  constructor(private opts: SidecarOptions) {}

  /** 获取当前状态 */
  getStatus(): SidecarStatus {
    return this.status
  }

  /** 更新状态并通知 */
  private setStatus(s: SidecarStatus): void {
    this.status = s
    this.opts.onStatusChange(s)
  }

  /** 启动 Go 子进程 */
  start(): void {
    if (this.process) return
    this.intentionalStop = false
    this.setStatus('starting')
    this.opts.onLog(`[sidecar] Starting ${this.opts.binaryPath}`)

    this.process = spawn(this.opts.binaryPath, [], {
      cwd: this.opts.workDir,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env },
    })

    this.process.stdout?.on('data', (data: Buffer) => {
      for (const line of data.toString().split('\n').filter(Boolean)) {
        this.opts.onLog(`[go:stdout] ${line}`)
      }
    })

    this.process.stderr?.on('data', (data: Buffer) => {
      for (const line of data.toString().split('\n').filter(Boolean)) {
        this.opts.onLog(`[go:stderr] ${line}`)
      }
    })

    this.process.on('exit', (code, signal) => {
      this.opts.onLog(`[sidecar] Process exited (code=${code}, signal=${signal})`)
      this.cleanup()
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
        this.setStatus('error')
      }
    }, 10_000)
  }

  /** 停止 Go 子进程 */
  stop(): void {
    this.intentionalStop = true
    this.cleanup()
    if (this.process) {
      this.setStatus('stopping')
      this.process.kill('SIGTERM')
      // 5 秒后强杀
      const forceKillTimer = setTimeout(() => {
        if (this.process) {
          this.opts.onLog('[sidecar] Force killing process')
          this.process.kill('SIGKILL')
        }
      }, 5000)
      this.process.on('exit', () => clearTimeout(forceKillTimer), { once: true } as any)
    } else {
      this.setStatus('stopped')
    }
  }

  /** 重启：停止后启动 */
  async restart(): Promise<void> {
    this.intentionalStop = true
    this.cleanup()
    if (this.process) {
      this.setStatus('stopping')
      this.process.kill('SIGTERM')
      await new Promise<void>((resolve) => {
        if (!this.process) { resolve(); return }
        this.process.on('exit', () => resolve(), { once: true } as any)
        setTimeout(resolve, 5000)
      })
    }
    this.process = null
    this.start()
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
    const req = http.get(`http://localhost:${this.opts.port}/health`, (res) => {
      if (res.statusCode === 200 && this.status === 'starting') {
        this.opts.onLog('[sidecar] Health check passed, service is running')
        if (this.startTimeout) {
          clearTimeout(this.startTimeout)
          this.startTimeout = null
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
    if (this.restartCount < this.maxRestarts) {
      this.restartCount++
      const delay = this.restartCount * 2000
      this.opts.onLog(`[sidecar] Auto-restart attempt ${this.restartCount}/${this.maxRestarts} in ${delay}ms`)
      this.setStatus('error')
      setTimeout(() => {
        this.process = null
        this.start()
      }, delay)
    } else {
      this.opts.onLog('[sidecar] Max restart attempts reached, marking as error')
      this.setStatus('error')
    }
  }
}
