import { ipcMain } from 'electron'
import fs from 'node:fs'
import net from 'node:net'
import {
  deriveEndpoints,
  fillMaskedSecrets,
  maskSensitiveFields,
  readManagedConfig,
  sanitizeConfig,
  validateConfig,
  type AppConfig,
} from './config'
import type { AdminTokenStore } from './admin-auth'
import type { SidecarManager } from './sidecar'
import { Channels } from '../shared/channels'
import type { RuntimeInfo, ServiceEndpoints } from '../shared/types'

/** 日志缓冲（最近 500 行） */
const logBuffer: string[] = []
const MAX_LOG_LINES = 500

/** 追加一行日志，超出上限时丢弃最旧行（供服务页实时展示） */
export function addLog(line: string): void {
  logBuffer.push(line)
  if (logBuffer.length > MAX_LOG_LINES) logBuffer.shift()
}

/** 取当前日志缓冲快照（服务页首屏拉取用） */
export function getLogBuffer(): string[] {
  return [...logBuffer]
}

/** registerIpcHandlers 所需的外部协作者，由主入口装配（便于单测替换） */
export interface IpcDeps {
  /** 仅用于读取状态与运行时长；启停走下面两个回调 */
  sidecar: SidecarManager
  /** 重读配置→更新运行快照→启动（配置无效时由主入口报错） */
  startService: () => void
  /** 同上，但等待旧进程退出后再启动 */
  restartService: () => Promise<void>
  configPath: string
  envPath: string
  /** sidecar 工作目录，用于把 database.path 解析成绝对路径 */
  workDir: string
  auth: AdminTokenStore
  /** 当前正在运行的进程所使用的配置快照；无有效快照时返回 null */
  getRuntimeConfig: () => AppConfig | null
  /** 配置已保存但尚未重启生效 */
  isConfigStale: () => boolean
  /** 落盘新配置并由主入口更新 stale 标记 / 失效 token */
  persistConfig: (config: AppConfig) => void
}

/** 计算概览页需要的运行监控信息 */
function buildRuntimeInfo(deps: IpcDeps): RuntimeInfo {
  const config = deps.getRuntimeConfig()
  const endpoints: ServiceEndpoints = config
    ? deriveEndpoints(config, deps.workDir)
    : { apiPort: 0, apiBaseUrl: '', adminBaseUrl: '', adminOrigin: '', dbPath: '' }

  let dbExists = false
  let dbSizeBytes = 0
  if (endpoints.dbPath) {
    try {
      dbExists = fs.existsSync(endpoints.dbPath)
      dbSizeBytes = dbExists ? fs.statSync(endpoints.dbPath).size : 0
    } catch {
      dbExists = false
      dbSizeBytes = 0
    }
  }

  return {
    status: deps.sidecar.getStatus(),
    uptimeMs: deps.sidecar.getUptimeMs(),
    endpoints,
    dbExists,
    dbSizeBytes,
    configStale: deps.isConfigStale(),
  }
}

/** 注册所有 IPC handler */
export function registerIpcHandlers(deps: IpcDeps): void {
  ipcMain.handle(Channels.GET_SERVICE_STATUS, () => {
    return { status: deps.sidecar.getStatus() }
  })

  ipcMain.handle(Channels.START_SERVICE, () => {
    deps.startService()
    return { ok: true }
  })

  ipcMain.handle(Channels.STOP_SERVICE, () => {
    deps.sidecar.stop()
    return { ok: true }
  })

  ipcMain.handle(Channels.RESTART_SERVICE, async () => {
    await deps.restartService()
    return { ok: true }
  })

  ipcMain.handle(Channels.GET_CONFIG, () => {
    const config = readManagedConfig(deps.configPath, deps.envPath)
    if (!config) {
      return { error: 'config.yaml 不存在或结构不完整，请检查各配置段与必填键' }
    }
    // 密钥不出主进程（ADR-0012 §3）：界面只见掩码空串
    return { config: maskSensitiveFields(config) }
  })

  ipcMain.handle(Channels.SAVE_CONFIG, (_event, payload: unknown) => {
    const sanitized = sanitizeConfig(payload)
    if ('errors' in sanitized) {
      return { errors: sanitized.errors }
    }
    // 掩码回填必须在业务校验之前：GET_CONFIG 把密钥掩成空串，空串在此表示「保持磁盘现值」
    const onDisk = readManagedConfig(deps.configPath, deps.envPath)
    if (!onDisk) {
      return { error: '无法读取当前配置，请先检查 config.yaml 格式是否正确' }
    }
    const config = fillMaskedSecrets(sanitized.config, onDisk)
    const businessErrors = validateConfig(config)
    if (businessErrors.length > 0) {
      return { errors: businessErrors }
    }
    deps.persistConfig(config)
    // 后端只在启动时读一次配置，保存后必须重启才生效——把这件事告诉界面而不是假装已生效
    return { ok: true, needRestart: true }
  })

  ipcMain.handle(Channels.GET_LOGS, () => {
    return { lines: getLogBuffer() }
  })

  ipcMain.handle(Channels.GET_RUNTIME_INFO, () => buildRuntimeInfo(deps))

  /** 对回环做一次真实 connect：连得上说明端口被占（无论被谁占） */
  async function probePort(port: number): Promise<boolean> {
    return new Promise((resolve) => {
      let settled = false
      const socket = net.connect({ port, host: '127.0.0.1' })
      const settle = (inUse: boolean): void => {
        if (settled) return
        settled = true
        socket.destroy()
        resolve(inUse)
      }
      socket.setTimeout(500)
      socket.on('connect', () => settle(true))
      socket.on('timeout', () => settle(false))
      socket.on('error', () => settle(false))
    })
  }

  ipcMain.handle(Channels.CHECK_PORT, async () => {
    const config = deps.getRuntimeConfig()
    if (!config) {
      return { port: 0, inUse: false }
    }
    return { port: config.server.port, inUse: await probePort(config.server.port) }
  })

  // renderer 拿到 token 即可调用 /admin/api/*；force 用于被拒后重取
  ipcMain.handle(Channels.GET_ADMIN_TOKEN, async (_event, force?: unknown) => {
    const result = await deps.auth.obtain(force === true)
    return result
  })
}

/** 注销所有 IPC handler（应用退出时调用） */
export function unregisterIpcHandlers(): void {
  for (const channel of Object.values(Channels)) {
    ipcMain.removeHandler(channel)
  }
}
