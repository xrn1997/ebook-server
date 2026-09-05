/**
 * IPC 调用封装。在 Electron 环境中通过 window.electronAPI 调用 Main Process；
 * 在浏览器环境中（开发调试）返回模拟数据。
 */

function getAPI(): ElectronAPI | null {
  return window.electronAPI ?? null
}

export async function getServiceStatus(): Promise<string> {
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

export async function getConfig(): Promise<Record<string, unknown> | null> {
  const api = getAPI()
  if (!api) return null
  const result = await api.getConfig()
  return result.config ?? null
}

export async function saveConfig(config: unknown): Promise<{ ok?: boolean; errors?: string[] }> {
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

export function onStatusChange(callback: (status: string) => void): (() => void) | null {
  const api = getAPI()
  if (!api) return null
  return api.onStatusChange((data) => callback(data.status))
}

export function onLogLine(callback: (line: string) => void): (() => void) | null {
  const api = getAPI()
  if (!api) return null
  return api.onLogLine((data) => callback(data.line))
}
