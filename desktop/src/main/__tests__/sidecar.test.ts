import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { EventEmitter } from 'node:events'
import { SidecarManager, type SidecarStatus } from '../sidecar'

// Mock child_process
vi.mock('node:child_process', () => {
  const mockProcess = new EventEmitter() as EventEmitter & {
    pid: number
    kill: ReturnType<typeof vi.fn>
    stdout: EventEmitter
    stderr: EventEmitter
  }
  mockProcess.pid = 12345
  mockProcess.kill = vi.fn(() => {
    mockProcess.emit('exit', 0)
    return true
  })
  mockProcess.stdout = new EventEmitter()
  mockProcess.stderr = new EventEmitter()

  return {
    spawn: vi.fn(() => mockProcess),
    __mockProcess: mockProcess,
  }
})

// Mock http
vi.mock('node:http', () => ({
  get: vi.fn(),
}))

describe('SidecarManager', () => {
  let manager: SidecarManager

  beforeEach(() => {
    manager = new SidecarManager({
      binaryPath: '/fake/ebook-server',
      workDir: '/fake/workdir',
      port: 9090,
      onStatusChange: vi.fn(),
      onLog: vi.fn(),
    })
  })

  afterEach(() => {
    manager.stop()
    vi.restoreAllMocks()
  })

  it('初始状态为 stopped', () => {
    expect(manager.getStatus()).toBe('stopped')
  })

  it('getStatus 返回当前状态', () => {
    expect(manager.getStatus()).toBe('stopped')
  })
})
