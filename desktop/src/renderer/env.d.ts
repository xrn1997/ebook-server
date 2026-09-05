/// <reference types="vite/client" />

interface ElectronAPI {
  getServiceStatus: () => Promise<{ status: string }>
  startService: () => Promise<{ ok: boolean }>
  stopService: () => Promise<{ ok: boolean }>
  restartService: () => Promise<{ ok: boolean }>
  getConfig: () => Promise<{ config?: Record<string, unknown>; error?: string }>
  saveConfig: (config: unknown) => Promise<{ ok?: boolean; errors?: string[] }>
  getLogs: () => Promise<{ lines: string[] }>
  onStatusChange: (callback: (data: { status: string }) => void) => () => void
  onLogLine: (callback: (data: { line: string }) => void) => () => void
}

interface Window {
  electronAPI?: ElectronAPI
}
