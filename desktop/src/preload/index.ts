import { contextBridge, ipcRenderer } from 'electron'

/** 暴露给 Renderer 的 IPC API（通过 window.electronAPI 访问） */
const electronAPI = {
  // 服务控制
  getServiceStatus: () => ipcRenderer.invoke('get-service-status'),
  startService: () => ipcRenderer.invoke('start-service'),
  stopService: () => ipcRenderer.invoke('stop-service'),
  restartService: () => ipcRenderer.invoke('restart-service'),

  // 配置管理
  getConfig: () => ipcRenderer.invoke('get-config'),
  saveConfig: (config: unknown) => ipcRenderer.invoke('save-config', config),

  // 日志
  getLogs: () => ipcRenderer.invoke('get-logs'),

  // 事件监听（Main → Renderer）
  onStatusChange: (callback: (data: { status: string }) => void) => {
    const handler = (_event: unknown, data: { status: string }) => callback(data)
    ipcRenderer.on('status-changed', handler)
    return () => ipcRenderer.removeListener('status-changed', handler)
  },
  onLogLine: (callback: (data: { line: string }) => void) => {
    const handler = (_event: unknown, data: { line: string }) => callback(data)
    ipcRenderer.on('log-line', handler)
    return () => ipcRenderer.removeListener('log-line', handler)
  },
}

contextBridge.exposeInMainWorld('electronAPI', electronAPI)

export type ElectronAPI = typeof electronAPI
