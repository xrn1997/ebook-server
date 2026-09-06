/// <reference types="vite/client" />
import type { ElectronAPI } from '../shared/types'

declare global {
  /** preload 通过 contextBridge 暴露的 IPC API，契约在 shared/types 单一定义 */
  interface Window {
    electronAPI?: ElectronAPI
  }
}

export {}
