/**
 * IPC 调用封装。在 Electron 环境中通过 window.electronAPI 调用 Main Process；
 * 在浏览器环境中（开发调试）返回 null，由调用方决定如何降级——
 * 不在此处伪造端口或数据，否则浏览器里看到的「正常」会掩盖真实链路的问题。
 */
import type {
  AdminTokenPayload,
  AppConfig,
  ElectronAPI,
  GetConfigResult,
  PortCheckResult,
  RuntimeInfo,
  SaveConfigResult,
  ServiceError,
  ServiceStatus,
} from '../../shared/types'

function getAPI(): ElectronAPI | null {
  return window.electronAPI ?? null
}

/** 主进程未就绪（如浏览器里跑 renderer）时没有状态可言，用 'unknown' 表达 */
export async function getServiceStatus(): Promise<ServiceStatus | 'unknown'> {
  const api = getAPI()
  if (!api) return 'unknown'
  const result = await api.getServiceStatus()
  return result.status
}

export async function startService(): Promise<void> {
  await getAPI()?.startService()
}

export async function stopService(): Promise<void> {
  await getAPI()?.stopService()
}

export async function restartService(): Promise<void> {
  await getAPI()?.restartService()
}

/**
 * 读取受管配置，一次调用同时拿到配置与失败原因。
 * 此前拆成 getConfig + getConfigError 两个函数，每个都要重新 invoke 并让主进程
 * 重读一遍磁盘文件；页面挂载时还真的两个都调了。
 */
export async function loadConfig(): Promise<{ config: AppConfig | null; error: string | null }> {
  const api = getAPI()
  if (!api) return { config: null, error: '非 Electron 环境，无法读取受管配置' }
  const result: GetConfigResult = await api.getConfig()
  return 'config' in result ? { config: result.config, error: null } : { config: null, error: result.error }
}

export async function saveConfig(config: AppConfig): Promise<SaveConfigResult> {
  const api = getAPI()
  if (!api) return { errors: ['非 Electron 环境'] }
  return api.saveConfig(config)
}

export async function getLogs(): Promise<string[]> {
  const api = getAPI()
  if (!api) return []
  const result = await api.getLogs()
  return result.lines
}

/** 运行监控信息（端口、数据库大小、运行时长、配置是否需要重启） */
export async function getRuntimeInfo(): Promise<RuntimeInfo | null> {
  const api = getAPI()
  if (!api) return null
  return api.getRuntimeInfo()
}

/** 端口占用探测：启动前检查端口是否可用 */
export async function checkPort(): Promise<PortCheckResult | null> {
  const api = getAPI()
  if (!api) return null
  return api.checkPort()
}

/** 向主进程索取后台 JWT；force 表示上一次调用被判定为令牌不被接受 */
export async function getAdminToken(force = false): Promise<AdminTokenPayload> {
  const api = getAPI()
  if (!api) return { error: '非 Electron 环境，无法取得管理端令牌' }
  return api.getAdminToken(force)
}

export function onStatusChange(callback: (status: ServiceStatus) => void): (() => void) | null {
  const api = getAPI()
  if (!api) return null
  return api.onStatusChange((data) => callback(data.status))
}

export function onLogLine(callback: (line: string) => void): (() => void) | null {
  const api = getAPI()
  if (!api) return null
  return api.onLogLine((data: { line: string }) => callback(data.line))
}

/** 订阅主进程报告的服务失败（端口被占用、配置无效、自动登录失败等） */
export function onServiceError(callback: (error: ServiceError) => void): (() => void) | null {
  const api = getAPI()
  if (!api) return null
  return api.onServiceError((data: ServiceError) => callback(data))
}
