import { contextBridge, ipcRenderer } from 'electron'
import { Channels, Events } from '../shared/channels'
import type { ElectronAPI, ServiceError, ServiceStatus } from '../shared/types'

/** 订阅 Main 推送的事件，返回取消订阅函数 */
function subscribe<T>(event: string, callback: (data: T) => void): () => void {
  const handler = (_event: unknown, data: T) => callback(data)
  ipcRenderer.on(event, handler)
  return () => ipcRenderer.removeListener(event, handler)
}

/**
 * 暴露给 Renderer 的 IPC API（通过 window.electronAPI 访问）。
 * 只转发固定通道，不把 ipcRenderer 本身交给页面——renderer 一旦被注入脚本
 * 就能借通用 invoke 调用任意通道。
 *
 * 实现受 shared/types 的 ElectronAPI 契约约束：通道漏了、多了或签名不符都编译不过。
 */
const electronAPI: ElectronAPI = {
  // 服务控制
  getServiceStatus: () => ipcRenderer.invoke(Channels.GET_SERVICE_STATUS),
  startService: () => ipcRenderer.invoke(Channels.START_SERVICE),
  stopService: () => ipcRenderer.invoke(Channels.STOP_SERVICE),
  restartService: () => ipcRenderer.invoke(Channels.RESTART_SERVICE),

  // 配置管理
  getConfig: () => ipcRenderer.invoke(Channels.GET_CONFIG),
  saveConfig: (config: unknown) => ipcRenderer.invoke(Channels.SAVE_CONFIG, config),

  // 日志
  getLogs: () => ipcRenderer.invoke(Channels.GET_LOGS),

  // 运行监控（端口、数据库大小、运行时长都只在主进程知道）
  getRuntimeInfo: () => ipcRenderer.invoke(Channels.GET_RUNTIME_INFO),

  // 端口占用探测（服务页启动前检查）
  checkPort: () => ipcRenderer.invoke(Channels.CHECK_PORT),

  // 后台 JWT：由主进程用受管配置自动登录后下发，凭据不进 renderer
  getAdminToken: (force?: boolean) => ipcRenderer.invoke(Channels.GET_ADMIN_TOKEN, force === true),

  // 事件监听（Main → Renderer）
  onStatusChange: (callback: (data: { status: ServiceStatus }) => void) =>
    subscribe(Events.STATUS_CHANGED, callback),
  onLogLine: (callback: (data: { line: string }) => void) => subscribe(Events.LOG_LINE, callback),
  onServiceError: (callback: (data: ServiceError) => void) =>
    subscribe(Events.SERVICE_ERROR, callback),
}

contextBridge.exposeInMainWorld('electronAPI', electronAPI)
