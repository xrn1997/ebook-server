import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { EventEmitter } from 'node:events'
import { SidecarManager, type SidecarStatus } from '../sidecar'
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

vi.mock('node:path', () => ({
  default: { resolve: (p: string) => p },
  resolve: (p: string) => p,
}))

/** Create a fresh mock ChildProcess (EventEmitter with kill/stdout/stderr) */
function createMockProcess() {
  const proc = new EventEmitter() as EventEmitter & {
    pid: number
    kill: ReturnType<typeof vi.fn>
    stdout: EventEmitter
    stderr: EventEmitter
  }
  proc.pid = Math.floor(Math.random() * 100000)
  proc.kill = vi.fn(() => {
    // Simulate exit when killed
    queueMicrotask(() => proc.emit('exit', null, 'SIGTERM'))
    return true
  })
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
  let manager: SidecarManager
  let mockProc: ReturnType<typeof createMockProcess>

  beforeEach(() => {
    vi.useFakeTimers()
    onStatusChange = vi.fn()
    onLog = vi.fn()
    mockProc = createMockProcess()
    vi.mocked(spawn).mockReturnValue(mockProc as any)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  function createManager(httpSimulator?: (req: EventEmitter) => void): SidecarManager {
    vi.mocked(http.get).mockImplementation(createHttpMock(httpSimulator) as any)
    manager = new SidecarManager({
      binaryPath: '/fake/ebook-server',
      workDir: '/fake/workdir',
      port: 9090,
      onStatusChange,
      onLog,
    })
    return manager
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
    spawn.mockClear()
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
    spawn.mockClear()
    manager.start()
    expect(spawn).not.toHaveBeenCalled()
    expect(manager.getStatus()).toBe('running')
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
  })

  // ── stop() ─────────────────────────────────────────────────────

  it('stop() sends SIGTERM and transitions to stopped after exit', () => {
    createManager()
    manager.start()
    manager.stop()
    expect(mockProc.kill).toHaveBeenCalledWith('SIGTERM')
    expect(manager.getStatus()).toBe('stopping')

    // Simulate process exit
    mockProc.emit('exit', 0, null)
    expect(manager.getStatus()).toBe('stopped')
  })

  it('stop() when no process is running just sets stopped', () => {
    createManager()
    manager.stop()
    expect(manager.getStatus()).toBe('stopped')
  })

  // ── restart() ──────────────────────────────────────────────────

  it('restart() stops then starts', async () => {
    createManager()
    manager.start()
    expect(manager.getStatus()).toBe('starting')

    const restartPromise = manager.restart()
    // The old process should be killed (kill emits exit via queueMicrotask)
    expect(mockProc.kill).toHaveBeenCalledWith('SIGTERM')

    // Await the restart — mockProc.kill's queueMicrotask will fire the exit event
    await restartPromise

    // A new process should have been spawned
    expect(spawn).toHaveBeenCalledTimes(2)
  })

  // ── Crash recovery ─────────────────────────────────────────────

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
  })
})
