/**
 * Main ↔ Preload ↔ Renderer 的 IPC 通道名。
 * 三个进程各自编译，通道名写错的后果是界面静默收不到数据，所以这份契约只维护一处。
 */

/** Renderer 主动 invoke 的请求通道 */
export const Channels = {
  GET_SERVICE_STATUS: 'get-service-status',
  RESTART_SERVICE: 'restart-service',
  STOP_SERVICE: 'stop-service',
  START_SERVICE: 'start-service',
  GET_CONFIG: 'get-config',
  SAVE_CONFIG: 'save-config',
  GET_LOGS: 'get-logs',
  GET_RUNTIME_INFO: 'get-runtime-info',
  CHECK_PORT: 'check-port',
  GET_ADMIN_TOKEN: 'get-admin-token',
} as const

/** 仅由 Main 向 Renderer 推送的事件通道（不能 removeHandler，故与 Channels 分开） */
export const Events = {
  STATUS_CHANGED: 'status-changed',
  LOG_LINE: 'log-line',
  SERVICE_ERROR: 'service-error',
} as const
