/**
 * 跨进程共享的数据契约（Main / Preload / Renderer 三方都引用）。
 * 放在 shared 目录是为了让 renderer 不必 import 主进程模块就能拿到同一份类型——
 * 两边各自声明同名 interface 迟早会漂移。
 */

/** 应用配置类型（对应 config.yaml 结构） */
export interface AppConfig {
  server: { port: number; mode: string }
  database: { path: string }
  jwt: { secret: string; expire_min: number }
  smtp: { host: string; port: number; username: string; password: string; from: string; insecure: boolean }
  admin: { username: string; password: string; jwt_secret: string; expire_min: number; listen_addr: string; listen_port: number }
  upload: { dir: string }
  api_docs: { enabled: boolean }
}

/** 由配置派生的服务端点（renderer 不得自己猜端口） */
export interface ServiceEndpoints {
  /** 公开 API 端口（sidecar 健康检查用，恒连回环） */
  apiPort: number
  /** 公开 API 基础地址（公开端口绑 0.0.0.0，回环必可达） */
  apiBaseUrl: string
  /** 后台 API 基址，已把通配监听地址换算成可连接地址 */
  adminBaseUrl: string
  /** 后台页面基址（不含 /admin/api，供概览页展示） */
  adminOrigin: string
  /** SQLite 文件绝对路径（运行监控展示） */
  dbPath: string
}

/** check-port 的响应：主进程对真实端口做一次 connect 探测 */
export interface PortCheckResult {
  port: number
  inUse: boolean
}

/** 后端启动/运行失败的原因分类，供界面给出可读提示 */
export type ServiceErrorReason =
  | 'port-conflict'
  | 'startup-timeout'
  | 'spawn-failed'
  | 'crashed'
  | 'config-invalid'
  | 'admin-login-failed'

/** 服务失败提示：原因与文案都由主进程给出（renderer 无从判断端口冲突等底层原因） */
export interface ServiceError {
  reason: ServiceErrorReason
  message: string
}

/** 后端统一响应信封（ADR-0001）：HTTP 状态恒为 200，成败由 body.code 表述 */
export interface Envelope<T = unknown> {
  code?: string
  error?: string
  data?: T
}

/**
 * Sidecar 子进程状态。主进程上报、renderer 展示的唯一状态词汇表；
 * 此前主进程与 renderer 各写一份同名类型，迟早漂移。
 */
export type ServiceStatus = 'stopped' | 'starting' | 'running' | 'stopping' | 'error'

/** 概览页需要的运行监控信息 */
export interface RuntimeInfo {
  status: ServiceStatus
  uptimeMs: number
  endpoints: ServiceEndpoints
  dbExists: boolean
  dbSizeBytes: number
  /** 已保存的配置与正在运行的进程不一致，需重启才生效 */
  configStale: boolean
}

/** Main → Renderer 的后台令牌推送 */
export interface AdminTokenPayload {
  token?: string
  error?: string
}

/** get-config 的响应 */
export type GetConfigResult = { config: AppConfig } | { error: string }

/** save-config 的响应 */
export type SaveConfigResult = { ok: true; needRestart: true } | { errors: string[] }

/**
 * 令牌不被接受的业务码（ADR-0001 信封）：无令牌/令牌无效/登录过期。
 * 主进程与渲染层都要判断「是否该重新登录」，而渲染层不能 import 主进程模块
 * （会把 node:http 等打进浏览器包），故契约放 shared。
 */
export const AUTH_FAILURE_CODES: readonly string[] = ['A0403', 'A0240', 'A0230']

/** 一次后台 API 响应的业务码是否表示令牌不被接受 */
export function isAdminAuthFailure(code?: string): boolean {
  return code !== undefined && AUTH_FAILURE_CODES.includes(code)
}

/**
 * preload 通过 contextBridge 暴露给 renderer 的 IPC API（window.electronAPI）。
 * 契约在此单一定义：preload 以它约束实现（多了少了都编译不过），renderer 的
 * env.d.ts 引用它声明 window 类型。此前 preload 与 env.d.ts 各写一份。
 */
export interface ElectronAPI {
  getServiceStatus: () => Promise<{ status: ServiceStatus }>
  startService: () => Promise<{ ok: boolean }>
  stopService: () => Promise<{ ok: boolean }>
  restartService: () => Promise<{ ok: boolean }>
  getConfig: () => Promise<GetConfigResult>
  saveConfig: (config: unknown) => Promise<SaveConfigResult>
  getLogs: () => Promise<{ lines: string[] }>
  getRuntimeInfo: () => Promise<RuntimeInfo | null>
  checkPort: () => Promise<PortCheckResult | null>
  getAdminToken: (force?: boolean) => Promise<AdminTokenPayload>
  onStatusChange: (callback: (data: { status: ServiceStatus }) => void) => () => void
  onLogLine: (callback: (data: { line: string }) => void) => () => void
  onServiceError: (callback: (data: ServiceError) => void) => () => void
}
