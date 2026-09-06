import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { EventEmitter } from 'node:events'
import { SidecarManager } from '../sidecar'
import type { ServiceErrorReason, ServiceStatus } from '../../shared/types'
import { spawn } from 'node:child_process'
import http from 'node:http'

vi.mock('node:child_process', () => ({
  spawn: vi.fn(),
}))

vi.mock('node:http', () => {
  const get = vi.fn()
  return {
    default: { get },
    get,
  }
})

/** Create a fresh mock ChildProcess (EventEmitter with kill/stdin/stdout/stderr) */
function createMockProcess() {
  const proc = new EventEmitter() as EventEmitter & {
    pid: number
    kill: ReturnType<typeof vi.fn>
    stdin: { write: ReturnType<typeof vi.fn>; end: ReturnType<typeof vi.fn>; on: ReturnType<typeof vi.fn> }
    stdout: EventEmitter
    stderr: EventEmitter
  }
  proc.pid = Math.floor(Math.random() * 100000)
  proc.kill = vi.fn(() => {
    // Simulate exit when killed
    queueMicrotask(() => proc.emit('exit', null, 'SIGTERM'))
    return true
  })
  proc.stdin = {
    write: vi.fn(() => true),
    end: vi.fn(),
    on: vi.fn(),
  }
  proc.stdout = new EventEmitter()
  proc.stderr = new EventEmitter()
  return proc
}

/**
 * Create a mock http.get that returns a controllable request EventEmitter.
 * `simulator` is called with the request EventEmitter so tests can trigger
 * response/error events on it.
 */
function createHttpMock(simulator?: (req: EventEmitter) => void) {
  return vi.fn((_url: string, cb?: (res: { statusCode: number }) => void) => {
    const req = new EventEmitter() as EventEmitter & {
      setTimeout: ReturnType<typeof vi.fn>
      destroy: ReturnType<typeof vi.fn>
    }
    req.setTimeout = vi.fn()
    req.destroy = vi.fn()
    // If a simulator is provided, let it drive the mock
    if (simulator) {
      simulator(req)
    } else if (cb) {
      // Default: simulate a successful 200 response on next tick
      queueMicrotask(() => cb({ statusCode: 200 }))
    }
    return req
  })
}

describe('SidecarManager', () => {
  let onStatusChange: ReturnType<typeof vi.fn>
  let onLog: ReturnType<typeof vi.fn>
  let onError: ReturnType<typeof vi.fn>
  let manager: SidecarManager
  let mockProc: ReturnType<typeof createMockProcess>

  beforeEach(() => {
    vi.useFakeTimers()
    onStatusChange = vi.fn()
    onLog = vi.fn()
    onError = vi.fn()
    mockProc = createMockProcess()
    vi.mocked(spawn).mockReturnValue(mockProc as any)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  function createManager(
    httpSimulator?: (req: EventEmitter) => void,
    overrides: { port?: () => number; platform?: string } = {},
  ): SidecarManager {
    vi.mocked(http.get).mockImplementation(createHttpMock(httpSimulator) as any)
    manager = new SidecarManager({
      binaryPath: '/fake/ebook-server',
      workDir: '/fake/workdir',
      port: overrides.port ?? (() => 9090),
      // 显式给平台：否则测试跟着宿主平台变语义（Windows 走 taskkill 而非 SIGTERM）
      platform: overrides.platform ?? 'linux',
      onStatusChange,
      onLog,
      onError,
    })
    return manager
  }

  /** 取某次 spawn 的参数，用于区分后端二进制与 taskkill */
  function spawnCallArgs(index: number): unknown[] {
    return vi.mocked(spawn).mock.calls[index] as unknown[]
  }

  // ── Initial state ──────────────────────────────────────────────

  it('初始状态为 stopped', () => {
    createManager()
    expect(manager.getStatus()).toBe('stopped')
  })

  // ── start() ────────────────────────────────────────────────────

  it('start() transitions to starting and spawns process', () => {
    createManager()
    manager.start()
    expect(manager.getStatus()).toBe('starting')
    expect(spawn).toHaveBeenCalledOnce()
    expect(onStatusChange).toHaveBeenCalledWith('starting')
  })

  it('start() is no-op when already starting', () => {
    createManager()
    manager.start()
    vi.mocked(spawn).mockClear()
    manager.start()
    expect(spawn).not.toHaveBeenCalled()
    expect(manager.getStatus()).toBe('starting')
  })

  it('start() is no-op when already running', async () => {
    createManager()
    manager.start()
    // Simulate health check success
    await vi.advanceTimersByTimeAsync(1000) // trigger first health check
    expect(manager.getStatus()).toBe('running')
    vi.mocked(spawn).mockClear()
    manager.start()
    expect(spawn).not.toHaveBeenCalled()
    expect(manager.getStatus()).toBe('running')
  })

  it('二进制无法启动时报 spawn-failed 而不是抛出异常', () => {
    createManager()
    vi.mocked(spawn).mockImplementationOnce(() => {
      throw new Error('ENOENT')
    })
    manager.start()
    expect(manager.getStatus()).toBe('error')
    expect(onError).toHaveBeenCalledWith('spawn-failed', expect.stringContaining('ENOENT'))
  })

  it('spawn 异步 emit error（Windows 上 ENOENT 不抛同步异常）也归为 spawn-failed', () => {
    createManager()
    manager.start()
    mockProc.emit('error', new Error('ENOENT'))
    expect(manager.getStatus()).toBe('error')
    expect(onError).toHaveBeenCalledWith('spawn-failed', expect.stringContaining('ENOENT'))
    expect(onStatusChange).not.toHaveBeenCalledWith('starting', expect.anything())
  })

  // ── Health check ───────────────────────────────────────────────

  it('health check success transitions to running and clears interval', async () => {
    const clearIntervalSpy = vi.spyOn(global, 'clearInterval')
    createManager()
    manager.start()
    expect(manager.getStatus()).toBe('starting')

    // Advance to first health check interval
    await vi.advanceTimersByTimeAsync(1000)
    expect(manager.getStatus()).toBe('running')

    // The health timer should have been cleared
    expect(clearIntervalSpy).toHaveBeenCalled()
    clearIntervalSpy.mockRestore()
  })

  it('health check failure keeps status as starting', () => {
    createManager((_req) => {
      // Never call the callback — simulates connection refused
    })
    manager.start()
    vi.advanceTimersByTime(1000)
    expect(manager.getStatus()).toBe('starting')
  })

  // ── 端口来源（改配置后重启必须拨新端口） ────────────────────────

  it('健康检查使用 port() 当前的值', async () => {
    let port = 9090
    createManager((_req) => {}, { port: () => port })
    manager.start()
    vi.advanceTimersByTime(1000)
    expect(vi.mocked(http.get).mock.calls[0][0]).toContain(':9090/health')

    port = 9999
    manager.stop()
    // 真实时序：stdin 指令让旧进程退出、状态落回 stopped 后才允许下一次启动
    mockProc.emit('exit', 0, null)
    await vi.advanceTimersByTimeAsync(0)
    manager.start()
    vi.advanceTimersByTime(1000)
    const lastCall = vi.mocked(http.get).mock.calls.at(-1)
    expect(lastCall?.[0]).toContain(':9999/health')
  })

  // ── startTimeout ───────────────────────────────────────────────

  it('startTimeout kills process and sets error after 10s', () => {
    createManager((_req) => {
      // Health check never succeeds
    })
    manager.start()
    expect(manager.getStatus()).toBe('starting')

    vi.advanceTimersByTime(10_000)
    expect(manager.getStatus()).toBe('error')
    expect(mockProc.kill).toHaveBeenCalledWith('SIGTERM')
    expect(onError).toHaveBeenCalledWith('startup-timeout', expect.stringContaining('9090'))
  })

  it('后端输出端口占用信息时，立即定性为 port-conflict 而不是等超时', () => {
    createManager((_req) => {})
    manager.start()

    mockProc.stderr.emit('data', Buffer.from('listen tcp :9090: bind: address already in use\n'))

    expect(manager.getStatus()).toBe('error')
    expect(onError).toHaveBeenCalledWith('port-conflict', expect.stringContaining('被占用'))
    expect(onError).not.toHaveBeenCalledWith('startup-timeout', expect.anything())
    // 端口被占是确定性故障，不该演变成 3 次自动重启
    vi.advanceTimersByTime(30_000)
    expect(spawn).toHaveBeenCalledTimes(1)
    expect(manager.getStatus()).toBe('error')
  })

  it('端口被别的进程占用时，别人的 /health 也不能让我们变成 running', async () => {
    // 健康检查一律秒回 200（模拟占用该端口的另一进程）
    createManager()
    manager.start()

    mockProc.stdout.emit('data', Buffer.from('listen tcp 0.0.0.0:9090: bind: Only one usage of each socket address\n'))
    expect(manager.getStatus()).toBe('error')

    await vi.advanceTimersByTimeAsync(5000)
    expect(manager.getStatus()).toBe('error')
  })

  it('多字节 UTF-8 字符跨 chunk 到达时按行缓冲、不产生替换符', () => {
    createManager()
    manager.start()

    const line = '[GIN] | 200 | 中文评论内容'
    const bytes = Buffer.from(line + '\n', 'utf8')
    // 把「中」字（3 字节）从中间劈开：half 落在其首字节之后
    const half = bytes.indexOf(Buffer.from('中', 'utf8')[0]) + 1
    mockProc.stdout.emit('data', bytes.subarray(0, half))
    mockProc.stdout.emit('data', bytes.subarray(half))

    expect(onLog).toHaveBeenCalledWith(`[go:stdout] ${line}`)
  })

  it('半行输出先缓存，凑齐整行才转发与定性', () => {
    createManager()
    manager.start()

    mockProc.stdout.emit('data', Buffer.from('listen tcp :9090: bind: address'))
    expect(manager.getStatus()).toBe('starting')
    expect(onError).not.toHaveBeenCalled()

    mockProc.stdout.emit('data', Buffer.from(' already in use\n'))
    expect(manager.getStatus()).toBe('error')
    expect(onError).toHaveBeenCalledWith('port-conflict', expect.stringContaining('被占用'))
  })

  it('进程退出时兜底吐出未换行的残留输出', () => {
    createManager()
    manager.start()

    mockProc.stdout.emit('data', Buffer.from('panic: runtime error')) // 半行，无换行
    mockProc.emit('exit', 1, null)

    expect(onLog).toHaveBeenCalledWith('[go:stdout] panic: runtime error')
  })

  // ── stop()：优雅退出 = stdin 指令 → 宽限期 → 强杀 ───────────────

  it('stop() 先向 stdin 写退出指令并进入 stopping，进程退出后落回 stopped', () => {
    createManager()
    manager.start()
    manager.stop()
    expect(mockProc.stdin.write).toHaveBeenCalledWith('shutdown\n')
    expect(mockProc.stdin.end).toHaveBeenCalled()
    // Windows 上 taskkill 不带 /F 对无窗口控制台进程无效，因此温和阶段不再发信号
    expect(mockProc.kill).not.toHaveBeenCalled()
    expect(manager.getStatus()).toBe('stopping')

    mockProc.emit('exit', 0, null)
    expect(manager.getStatus()).toBe('stopped')
  })

  it('stop() when no process is running just sets stopped', () => {
    createManager()
    manager.stop()
    expect(manager.getStatus()).toBe('stopped')
  })

  it('宽限期内自行退出的进程不会被强杀', () => {
    createManager(undefined, { platform: 'win32' })
    manager.start()
    manager.stop()
    mockProc.emit('exit', 0, null)
    vi.advanceTimersByTime(6_000)
    // 优雅退出成功：连 taskkill 都不需要
    expect(spawn).toHaveBeenCalledTimes(1)
    expect(mockProc.kill).not.toHaveBeenCalled()
  })

  it('宽限期（5 秒）耗尽仍存活才强杀（taskkill /F /T）', () => {
    createManager(undefined, { platform: 'win32' })
    manager.start()
    const pid = mockProc.pid

    manager.stop()
    vi.advanceTimersByTime(4_999)
    expect(spawn).toHaveBeenCalledTimes(1) // 只有最初 spawn 后端那一次
    vi.advanceTimersByTime(1)
    expect(spawnCallArgs(1)[0]).toBe('taskkill')
    expect(spawnCallArgs(1)[1]).toEqual(['/F', '/T', '/PID', String(pid)])
  })

  it('taskkill spawn 异步报错（ENOENT）时兜底走 proc.kill', () => {
    createManager(undefined, { platform: 'win32' })
    manager.start()
    const pid = mockProc.pid

    // spawn 同步返回成功，ENOENT 走异步 error 事件：现有 try/catch 罩不住
    const killer = new EventEmitter()
    vi.mocked(spawn).mockImplementationOnce(() => killer as any)
    manager.stop()
    vi.advanceTimersByTime(5_000)

    expect(spawnCallArgs(1)[0]).toBe('taskkill')
    killer.emit('error', new Error('spawn taskkill ENOENT'))
    expect(mockProc.kill).toHaveBeenCalledWith('SIGKILL')
  })

  it('stopping 期间 start() 是 no-op（否则会抢尚未释放的端口、误报端口冲突）', () => {
    createManager()
    manager.start()
    manager.stop()
    expect(manager.getStatus()).toBe('stopping')

    vi.mocked(spawn).mockClear()
    manager.start()
    expect(spawn).not.toHaveBeenCalled()
    expect(manager.getStatus()).toBe('stopping')
  })

  it('stdin 写入抛错（进程已死）不阻断停止流程', () => {
    createManager()
    manager.start()
    mockProc.stdin.write.mockImplementation(() => {
      throw new Error('EPIPE')
    })
    expect(() => manager.stop()).not.toThrow()
    expect(manager.getStatus()).toBe('stopping')
    mockProc.emit('exit', 0, null)
    expect(manager.getStatus()).toBe('stopped')
  })

  // ── 崩溃自动重启 ───────────────────────────────────────────────

  it('restart() 停在退避窗口时不会把进程复活', () => {
    createManager((_req) => {})
    manager.start()
    // 崩溃 → 进入 2 秒退避
    mockProc.emit('exit', 1, null)
    expect(manager.getStatus()).toBe('error')

    // 用户在退避窗口内点「停止」
    manager.stop()
    expect(manager.getStatus()).toBe('stopped')

    vi.advanceTimersByTime(30_000)
    // 只有最初那一次 spawn，没有被自动重启拉起
    expect(spawn).toHaveBeenCalledTimes(1)
  })

  it('crash recovery auto-restarts up to max times', () => {
    createManager((_req) => {
      // Health never succeeds — each spawned process will time out
    })

    // First process crashes
    manager.start()
    mockProc.emit('exit', 1, null)
    expect(manager.getStatus()).toBe('error')
    expect(onLog).toHaveBeenCalledWith(expect.stringContaining('Auto-restart attempt 1'))

    // Advance to trigger restart, then let it crash again
    vi.advanceTimersByTime(2000)
    expect(manager.getStatus()).toBe('starting')
    const secondProc = vi.mocked(spawn).mock.results[1].value
    secondProc.emit('exit', 1, null)
    expect(onLog).toHaveBeenCalledWith(expect.stringContaining('Auto-restart attempt 2'))

    // Third crash
    vi.advanceTimersByTime(4000)
    const thirdProc = vi.mocked(spawn).mock.results[2].value
    thirdProc.emit('exit', 1, null)
    expect(onLog).toHaveBeenCalledWith(expect.stringContaining('Auto-restart attempt 3'))

    // Fourth crash — should give up
    vi.advanceTimersByTime(6000)
    const fourthProc = vi.mocked(spawn).mock.results[3].value
    fourthProc.emit('exit', 1, null)
    expect(manager.getStatus()).toBe('error')
    expect(onLog).toHaveBeenCalledWith(expect.stringContaining('Max restart attempts reached'))
  })

  it('crash recovery gives up after max restarts', () => {
    createManager((_req) => {})
    manager.start()

    // Crash 3 times
    for (let i = 0; i < 3; i++) {
      const proc = vi.mocked(spawn).mock.results[i].value
      proc.emit('exit', 1, null)
      const delay = (i + 1) * 2000
      vi.advanceTimersByTime(delay)
    }

    // 4th crash: should give up
    const lastProc = vi.mocked(spawn).mock.results[3].value
    lastProc.emit('exit', 1, null)
    expect(manager.getStatus()).toBe('error')
    expect(onLog).toHaveBeenCalledWith(
      expect.stringContaining('Max restart attempts reached')
    )
    // No more restarts
    expect(spawn).toHaveBeenCalledTimes(4) // 1 original + 3 restarts
    expect(onError).toHaveBeenLastCalledWith('crashed', expect.stringContaining('不再重试'))
  })

  // ── restart() ──────────────────────────────────────────────────

  it('restart() 等旧进程退出后再启动新进程', async () => {
    createManager()
    manager.start()
    expect(manager.getStatus()).toBe('starting')

    const restartPromise = manager.restart()
    // 优雅退出指令写入 stdin，而不是发信号
    expect(mockProc.stdin.write).toHaveBeenCalledWith('shutdown\n')

    // 旧进程退出（真实中由 Go 侧优雅关闭后自行退出）后才拉起新进程
    mockProc.emit('exit', 0, null)
    await restartPromise

    expect(spawn).toHaveBeenCalledTimes(2)
  })

  // ── 运行时长（概览页「运行时间」） ─────────────────────────────

  it('仅 running 状态给出运行时长', async () => {
    createManager()
    manager.start()
    expect(manager.getUptimeMs()).toBe(0)

    await vi.advanceTimersByTimeAsync(1000)
    expect(manager.getStatus()).toBe('running')

    await vi.advanceTimersByTimeAsync(5_000)
    expect(manager.getUptimeMs()).toBeGreaterThanOrEqual(5_000)

    manager.stop()
    mockProc.emit('exit', 0, null)
    expect(manager.getUptimeMs()).toBe(0)
  })

  it('状态回调按顺序反映生命周期', async () => {
    const seen: ServiceStatus[] = []
    createManager()
    manager = new SidecarManager({
      binaryPath: '/fake/ebook-server',
      workDir: '/fake/workdir',
      port: () => 9090,
      platform: 'linux',
      onStatusChange: (s) => seen.push(s),
      onLog,
      onError: (r: ServiceErrorReason) => onError(r),
    })
    manager.start()
    await vi.advanceTimersByTimeAsync(1000)
    manager.stop()
    mockProc.emit('exit', 0, null)
    expect(seen).toEqual(['starting', 'running', 'stopping', 'stopped'])
  })
})
