import { describe, it, expect, vi, beforeEach } from 'vitest'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import net from 'node:net'

/** ipcMain 打桩：把注册的 handler 收集起来，测试里直接调用它们 */
const handlers = new Map<string, (event: unknown, payload?: unknown) => unknown>()

vi.mock('electron', () => ({
  ipcMain: {
    handle: vi.fn((channel: string, fn: (event: unknown, payload?: unknown) => unknown) => {
      handlers.set(channel, fn)
    }),
    removeHandler: vi.fn(),
  },
}))

import { registerIpcHandlers, type IpcDeps } from '../ipc'
import { Channels } from '../../shared/channels'
import type { RuntimeInfo, PortCheckResult } from '../../shared/types'
import { AdminTokenStore } from '../admin-auth'
import { writeFullConfig, maskSensitiveFields, type AppConfig } from '../config'
import type { SidecarManager } from '../sidecar'

function validConfig(): AppConfig {
  return {
    server: { port: 9090, mode: 'release' },
    database: { path: 'ebook.db' },
    jwt: { secret: 'jwt-s', expire_min: 120 },
    smtp: { host: 'smtp.qq.com', port: 465, username: 'a@qq.com', password: 'smtp-s', from: 'a@qq.com', insecure: false },
    admin: { username: 'admin', password: 'adm-p', jwt_secret: 'adm-j', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
    upload: { dir: 'uploads' },
    api_docs: { enabled: false },
  }
}

/** 调起某个已注册的 handler */
function invoke<T = unknown>(channel: string, payload?: unknown): T {
  const fn = handlers.get(channel)
  if (!fn) throw new Error(`handler ${channel} 未注册`)
  return fn({}, payload) as T
}

describe('IPC 配置通道', () => {
  let dir: string
  let deps: IpcDeps & { persistConfig: ReturnType<typeof vi.fn> }

  beforeEach(() => {
    handlers.clear()
    dir = fs.mkdtempSync(path.join(os.tmpdir(), 'desktop-ipc-'))
    const configPath = path.join(dir, 'config.yaml')
    const envPath = path.join(dir, '.env')
    writeFullConfig(configPath, envPath, validConfig())

    const sidecar = {
      getStatus: () => 'running',
      getUptimeMs: () => 12_345,
      stop: vi.fn(),
    } as unknown as SidecarManager

    deps = {
      sidecar,
      startService: vi.fn(),
      restartService: vi.fn(async () => {}),
      configPath,
      envPath,
      workDir: dir,
      auth: new AdminTokenStore(() => null),
      getRuntimeConfig: () => validConfig(),
      isConfigStale: () => false,
      persistConfig: vi.fn(),
    }
    registerIpcHandlers(deps)
  })

  it('启停走主入口注入的流程，而不是直接操作子进程', () => {
    invoke(Channels.START_SERVICE)
    expect(deps.startService).toHaveBeenCalledOnce()

    invoke(Channels.STOP_SERVICE)
    expect(deps.sidecar.stop).toHaveBeenCalledOnce()
  })

  it('结构不完整的配置被拒绝且不落盘', () => {
    const before = fs.readFileSync(deps.configPath, 'utf-8')

    const result = invoke<{ errors?: string[] }>(Channels.SAVE_CONFIG, { server: { port: 9090 } })

    expect(result.errors).toContain('缺少配置段 smtp')
    expect(deps.persistConfig).not.toHaveBeenCalled()
    expect(fs.readFileSync(deps.configPath, 'utf-8')).toBe(before)
  })

  it('renderer 夹带的未知键不会写进后端配置文件', () => {
    invoke(Channels.SAVE_CONFIG, { ...validConfig(), evil: { rm: '/' } })

    expect(deps.persistConfig).toHaveBeenCalledTimes(1)
    const saved = deps.persistConfig.mock.calls[0][0] as Record<string, unknown>
    expect(saved).not.toHaveProperty('evil')
  })

  it('后台监听地址改成通配会被拒绝（ADR-0010）', () => {
    const result = invoke<{ errors?: string[] }>(
      Channels.SAVE_CONFIG,
      { ...validConfig(), admin: { ...validConfig().admin, listen_addr: '0.0.0.0' } },
    )

    expect(result.errors?.join(' ')).toContain('通配地址')
    expect(deps.persistConfig).not.toHaveBeenCalled()
  })

  it('保存成功时明确告知需要重启才生效', () => {
    const result = invoke<{ ok?: boolean; needRestart?: boolean }>(Channels.SAVE_CONFIG, validConfig())
    expect(result).toEqual({ ok: true, needRestart: true })
  })

  it('config.yaml 被手工改坏时返回错误而不是让主进程抛异常', () => {
    fs.writeFileSync(deps.configPath, 'server: 42\n', 'utf-8')

    const result = invoke<{ error?: string; config?: AppConfig }>(Channels.GET_CONFIG)

    expect(result.error).toContain('结构不完整')
    expect(result.config).toBeUndefined()
  })

  it('GET_CONFIG 返回掩码后的配置：敏感字段为空串、其余原样', () => {
    const result = invoke<{ config?: AppConfig }>(Channels.GET_CONFIG)
    expect(result.config?.jwt.secret).toBe('')
    expect(result.config?.smtp.password).toBe('')
    expect(result.config?.admin.password).toBe('')
    expect(result.config?.admin.jwt_secret).toBe('')
    expect(result.config?.server.port).toBe(9090)
  })

  it('SAVE_CONFIG 收到掩码配置时空串回填磁盘现值再落盘', () => {
    invoke(Channels.SAVE_CONFIG, maskSensitiveFields(validConfig()))
    expect(deps.persistConfig).toHaveBeenCalledTimes(1)
    const saved = deps.persistConfig.mock.calls[0][0] as AppConfig
    expect(saved.jwt.secret).toBe('jwt-s')
    expect(saved.smtp.password).toBe('smtp-s')
    expect(saved.admin.password).toBe('adm-p')
    expect(saved.admin.jwt_secret).toBe('adm-j')
  })

  it('SAVE_CONFIG 收到非空新值时按新值落盘', () => {
    invoke(Channels.SAVE_CONFIG, { ...validConfig(), jwt: { ...validConfig().jwt, secret: 'brand-new' } })
    const saved = deps.persistConfig.mock.calls[0][0] as AppConfig
    expect(saved.jwt.secret).toBe('brand-new')
  })

  it('运行监控给出端口、运行时长、数据库大小与是否需要重启', () => {
    fs.writeFileSync(path.join(dir, 'ebook.db'), Buffer.alloc(2048))

    const info = invoke<RuntimeInfo>(Channels.GET_RUNTIME_INFO)

    expect(info.status).toBe('running')
    expect(info.uptimeMs).toBe(12_345)
    expect(info.endpoints.apiPort).toBe(9090)
    expect(info.endpoints.adminBaseUrl).toBe('http://127.0.0.1:9091/admin/api')
    expect(info.dbExists).toBe(true)
    expect(info.dbSizeBytes).toBe(2048)
    expect(info.configStale).toBe(false)
  })

  it('未读到有效运行配置时端点为空而不是指向猜测的端口', () => {
    deps.getRuntimeConfig = () => null
    const info = invoke<RuntimeInfo>(Channels.GET_RUNTIME_INFO)
    expect(info.endpoints).toEqual({
      apiPort: 0,
      apiBaseUrl: '',
      adminBaseUrl: '',
      adminOrigin: '',
      dbPath: '',
    })
  })

  it('CHECK_PORT 探测被占端口返回 inUse: true', async () => {
    const server = net.createServer()
    await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
    const port = (server.address() as { port: number }).port
    deps.getRuntimeConfig = () => ({ ...validConfig(), server: { port, mode: 'release' } })
    try {
      const payload = await invoke<PortCheckResult>(Channels.CHECK_PORT)
      expect(payload).toEqual({ port, inUse: true })
    } finally {
      server.close()
    }
  })

  it('CHECK_PORT 配置缺失时返回 port 0 与 inUse: false', async () => {
    deps.getRuntimeConfig = () => null
    const payload = await invoke<PortCheckResult>(Channels.CHECK_PORT)
    expect(payload).toEqual({ port: 0, inUse: false })
  })
})
