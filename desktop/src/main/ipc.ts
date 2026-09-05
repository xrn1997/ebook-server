import { ipcMain, type BrowserWindow } from 'electron'
import fs from 'node:fs'
import {
  readFullConfig,
  writeFullConfig,
  validateConfig,
  type AppConfig,
} from './config'
import { SidecarManager } from './sidecar'
import { getConfigPath, getEnvPath } from './paths'

/** IPC 通道名常量 */
export const Channels = {
  GET_SERVICE_STATUS: 'get-service-status',
  RESTART_SERVICE: 'restart-service',
  STOP_SERVICE: 'stop-service',
  START_SERVICE: 'start-service',
  GET_CONFIG: 'get-config',
  SAVE_CONFIG: 'save-config',
  GET_LOGS: 'get-logs',
  STATUS_CHANGED: 'status-changed',
  LOG_LINE: 'log-line',
} as const

/** 日志缓冲（最近 500 行） */
const logBuffer: string[] = []
const MAX_LOG_LINES = 500

export function addLog(line: string): void {
  logBuffer.push(line)
  if (logBuffer.length > MAX_LOG_LINES) logBuffer.shift()
}

export function getLogBuffer(): string[] {
  return [...logBuffer]
}

/** 注册所有 IPC handler */
export function registerIpcHandlers(
  sidecar: SidecarManager,
  userDataPath: string,
): void {
  const configPath = getConfigPath(userDataPath)
  const envPath = getEnvPath(userDataPath)

  ipcMain.handle(Channels.GET_SERVICE_STATUS, () => {
    return { status: sidecar.getStatus() }
  })

  ipcMain.handle(Channels.START_SERVICE, () => {
    sidecar.start()
    return { ok: true }
  })

  ipcMain.handle(Channels.STOP_SERVICE, () => {
    sidecar.stop()
    return { ok: true }
  })

  ipcMain.handle(Channels.RESTART_SERVICE, async () => {
    await sidecar.restart()
    return { ok: true }
  })

  ipcMain.handle(Channels.GET_CONFIG, () => {
    if (!fs.existsSync(configPath)) {
      return { error: 'config.yaml not found' }
    }
    const config = readFullConfig(configPath, envPath)
    return { config }
  })

  ipcMain.handle(Channels.SAVE_CONFIG, (_event, config: AppConfig) => {
    const errors = validateConfig(config)
    if (errors.length > 0) {
      return { errors }
    }
    writeFullConfig(configPath, envPath, config)
    return { ok: true }
  })

  ipcMain.handle(Channels.GET_LOGS, () => {
    return { lines: getLogBuffer() }
  })
}

/** 注销所有 IPC handler（应用退出时调用） */
export function unregisterIpcHandlers(): void {
  for (const channel of Object.values(Channels)) {
    ipcMain.removeHandler(channel)
  }
}
