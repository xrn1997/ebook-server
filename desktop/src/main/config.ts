import yaml from 'js-yaml'
import fs from 'node:fs'
import { isIP } from 'node:net'
import path from 'node:path'
import type { AppConfig, ServiceEndpoints } from '../shared/types'

// 配置结构与端点形状由 shared 统一定义（renderer 侧要按同一份类型渲染表单）
export type { AppConfig, ServiceEndpoints }

/** .env 键值对 */
export type EnvVars = Record<string, string>

/** 支持的标量类型名（sanitize 与校验共用） */
type Scalar = 'number' | 'string' | 'boolean'

/** 一个 section 的键→类型表 */
type SectionSchema = Record<string, Scalar>

/**
 * config.yaml 的已知结构。校验与落盘都以它为准：
 * renderer 传入的未知键一律丢弃，避免把任意内容写进后端的配置文件。
 */
const SCHEMA: { [section: string]: SectionSchema } = {
  server: { port: 'number', mode: 'string' },
  database: { path: 'string' },
  jwt: { secret: 'string', expire_min: 'number' },
  smtp: {
    host: 'string',
    port: 'number',
    username: 'string',
    password: 'string',
    from: 'string',
    insecure: 'boolean',
  },
  admin: {
    username: 'string',
    password: 'string',
    jwt_secret: 'string',
    expire_min: 'number',
    listen_addr: 'string',
    listen_port: 'number',
  },
  upload: { dir: 'string' },
  api_docs: { enabled: 'boolean' },
}

/** 敏感字段映射：config.yaml 的 section.key ↔ .env 键。全部为两段路径，故不需要通用路径游走器 */
const SENSITIVE_FIELDS: Array<{ section: string; key: string; envKey: string }> = [
  { section: 'jwt', key: 'secret', envKey: 'JWT_SECRET' },
  { section: 'smtp', key: 'password', envKey: 'SMTP_PASSWORD' },
  { section: 'admin', key: 'password', envKey: 'ADMIN_PASSWORD' },
  { section: 'admin', key: 'jwt_secret', envKey: 'ADMIN_JWT_SECRET' },
]

/** 读路径掩码：密钥不出主进程（ADR-0012 §3），界面只见空串 */
export function maskSensitiveFields(config: AppConfig): AppConfig {
  const copy = JSON.parse(JSON.stringify(config)) as Record<string, Record<string, unknown>>
  for (const { section, key } of SENSITIVE_FIELDS) {
    copy[section][key] = ''
  }
  return copy as unknown as AppConfig
}

/** 写路径回填：掩码留下的空串视为「保持磁盘现值」 */
export function fillMaskedSecrets(incoming: AppConfig, onDisk: AppConfig): AppConfig {
  const copy = JSON.parse(JSON.stringify(incoming)) as Record<string, Record<string, unknown>>
  const disk = onDisk as unknown as Record<string, Record<string, unknown>>
  for (const { section, key } of SENSITIVE_FIELDS) {
    if (copy[section][key] === '' && disk[section][key] !== undefined && disk[section][key] !== null) {
      copy[section][key] = disk[section][key]
    }
  }
  return copy as unknown as AppConfig
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** 解析 .env 文件内容为键值对 */
export function parseEnv(content: string): EnvVars {
  const result: EnvVars = {}
  for (const line of content.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const eqIdx = trimmed.indexOf('=')
    if (eqIdx === -1) continue
    const key = trimmed.slice(0, eqIdx).trim()
    let value = trimmed.slice(eqIdx + 1).trim()
    // 去除首尾引号
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1)
    }
    result[key] = value
  }
  return result
}

/** 将键值对序列化为 .env 文件内容 */
export function serializeEnv(vars: EnvVars): string {
  return Object.entries(vars)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')
}

/** sanitizeConfig 的结果：要么给出已知形状的配置，要么给出结构性错误 */
export type SanitizeResult = { config: AppConfig } | { errors: string[] }

/**
 * 按 SCHEMA 收敛任意输入，返回已知形状的配置或结构性错误。
 * 为什么单独一步：renderer→main 是信任边界，落盘前必须保证既有正确形状、
 * 又不含未知键，否则一份畸形配置就能污染后端启动文件。
 */
export function sanitizeConfig(input: unknown): SanitizeResult {
  const errors: string[] = []
  if (!isRecord(input)) {
    return { errors: ['配置必须是对象'] }
  }

  const out: Record<string, Record<string, unknown>> = {}
  for (const [section, keys] of Object.entries(SCHEMA)) {
    const raw = input[section]
    if (!isRecord(raw)) {
      errors.push(`缺少配置段 ${section}`)
      continue
    }
    const picked: Record<string, unknown> = {}
    for (const [key, type] of Object.entries(keys)) {
      const value = raw[key]
      if (value === undefined || value === null) {
        errors.push(`${section}.${key} 缺失`)
        continue
      }
      // 数字输入框被清空会得到空串，按类型错误报出来，避免把 '' 落进配置
      const actual = typeof value
      if (actual !== type) {
        errors.push(`${section}.${key} 类型应为 ${type}，实际为 ${actual}`)
        continue
      }
      picked[key] = value
    }
    out[section] = picked
  }

  if (errors.length > 0) return { errors }
  return { config: out as unknown as AppConfig }
}

/** 只关心形状的读取路径（磁盘上的配置）用：形状不合就返回 null */
export function trySanitize(input: unknown): AppConfig | null {
  const result = sanitizeConfig(input)
  return 'config' in result ? result.config : null
}

/**
 * 将 AppConfig 拆分为 YAML 部分（非敏感）和 ENV 部分（敏感）。
 * 敏感字段（密码、密钥）从 yamlConfig 中移除，放入 envVars。
 */
export function splitConfigToEnv(config: AppConfig): { yamlConfig: Record<string, unknown>; envVars: EnvVars } {
  const yamlConfig = JSON.parse(JSON.stringify(config)) as Record<string, unknown>
  const envVars: EnvVars = {}

  for (const { section, key, envKey } of SENSITIVE_FIELDS) {
    const bucket = yamlConfig[section]
    if (isRecord(bucket) && key in bucket) {
      const value = bucket[key]
      if (value !== undefined && value !== '') {
        envVars[envKey] = String(value)
      }
      delete bucket[key]
    }
  }

  return { yamlConfig, envVars }
}

/**
 * 将 env 变量合并回 YAML 配置对象（还原敏感字段），供 GUI 回填。
 */
export function mergeConfigToYaml(yamlConfig: Record<string, unknown>, envVars: EnvVars): AppConfig {
  const merged = JSON.parse(JSON.stringify(yamlConfig)) as Record<string, unknown>

  for (const { section, key, envKey } of SENSITIVE_FIELDS) {
    const value = envVars[envKey]
    if (value === undefined) continue
    const bucket = isRecord(merged[section]) ? (merged[section] as Record<string, unknown>) : {}
    bucket[key] = value
    merged[section] = bucket
  }

  return merged as unknown as AppConfig
}

/**
 * 通配监听地址（绑定语义 = 所有网卡）。ADR-0012 §4：后台端口靠监听地址与公网隔离，
 * 这些值一律拒绝保存；它们也不是能拨号出去的目标，连接时一律换算回环。
 * 名单必须只有这一份——两份名单漂移时，[::] 会过校验却在连接时被当通配处理，
 * 等于把 ADR-0012 的禁令漏了一个洞。
 */
export const WILDCARD_LISTEN_ADDRS: readonly string[] = ['0.0.0.0', '::', '[::]', '*']

/**
 * 把「绑定地址」换算成「可连接地址」：通配监听不是能拨出去的目标，连它必须改用回环。
 * 填内网 IP 时按其本身连（该地址确实被绑定了）。
 */
export function resolveConnectHost(listenAddr: string): string {
  const addr = (listenAddr || '').trim()
  // 空串按「未配置」处理：后端缺省监听 127.0.0.1，连接也按回环走
  if (addr === '' || WILDCARD_LISTEN_ADDRS.includes(addr)) {
    return '127.0.0.1'
  }
  return addr
}

/**
 * 从配置计算访问后端所需的端点。
 * @param config 合并了 .env 的完整配置
 * @param workDir sidecar 工作目录（database.path 相对它解析）
 */
export function deriveEndpoints(config: AppConfig, workDir: string): ServiceEndpoints {
  return {
    apiPort: config.server.port,
    apiBaseUrl: `http://127.0.0.1:${config.server.port}`,
    adminBaseUrl: `http://${resolveConnectHost(config.admin.listen_addr)}:${config.admin.listen_port}/admin/api`,
    adminOrigin: `http://${resolveConnectHost(config.admin.listen_addr)}:${config.admin.listen_port}`,
    dbPath: path.resolve(workDir, config.database.path),
  }
}

/** 业务规则校验（假定已通过 sanitizeConfig 的形状校验），返回错误消息列表 */
export function validateConfig(config: AppConfig): string[] {
  const errors: string[] = []

  if (config.server.port < 1 || config.server.port > 65535) {
    errors.push('server.port 必须在 1-65535 之间')
  }
  if (config.admin.listen_port < 1 || config.admin.listen_port > 65535) {
    errors.push('admin.listen_port 必须在 1-65535 之间')
  }
  if (!config.jwt.secret) {
    errors.push('jwt.secret 不能为空')
  }
  if (!config.admin.jwt_secret) {
    errors.push('admin.jwt_secret 不能为空')
  }
  // ADR-0010：后台与公网靠监听地址做网络隔离。通配地址会把后台端口整个暴露出去，
  // 而文档给出的局域网用法是「填具体内网 IP」，因此这里只禁通配，不禁内网 IP。
  const listenAddr = (config.admin.listen_addr || '').trim()
  if (WILDCARD_LISTEN_ADDRS.includes(listenAddr)) {
    errors.push('admin.listen_addr 不允许使用通配地址（ADR-0010 网络隔离），请填 127.0.0.1 或具体内网 IP')
  }
  // 主机名一律拒绝：isIP 把「填错成域名」挡在保存前，否则后端启动时 DNS 解析失败
  // 或解析到公网地址，都会让 ADR-0010 的隔离假设悄悄失效
  if (
    !WILDCARD_LISTEN_ADDRS.includes(listenAddr) &&
    listenAddr !== '' &&
    listenAddr !== 'localhost' &&
    isIP(listenAddr) === 0
  ) {
    errors.push('admin.listen_addr 必须是 IP 地址或 localhost，不能填主机名')
  }
  if (config.smtp.host && (config.smtp.port < 1 || config.smtp.port > 65535)) {
    errors.push('smtp.port 必须在 1-65535 之间')
  }

  return errors
}

/** 从文件路径加载 config.yaml */
export function loadYamlFile(filePath: string): Record<string, unknown> {
  const content = fs.readFileSync(filePath, 'utf-8')
  return (yaml.load(content) as Record<string, unknown>) || {}
}

/** 将配置对象写入 config.yaml 文件 */
export function saveYamlFile(filePath: string, data: Record<string, unknown>): void {
  fs.mkdirSync(path.dirname(filePath), { recursive: true })
  fs.writeFileSync(filePath, yaml.dump(data, { lineWidth: 120 }), 'utf-8')
}

/** 从文件路径加载 .env */
export function loadEnvFile(filePath: string): EnvVars {
  if (!fs.existsSync(filePath)) return {}
  return parseEnv(fs.readFileSync(filePath, 'utf-8'))
}

/** 将 env 变量写入 .env 文件 */
export function saveEnvFile(filePath: string, vars: EnvVars): void {
  fs.mkdirSync(path.dirname(filePath), { recursive: true })
  fs.writeFileSync(filePath, serializeEnv(vars), 'utf-8')
}

/**
 * 读取完整配置（合并 config.yaml + .env）。
 */
export function readFullConfig(yamlPath: string, envPath: string): AppConfig {
  const yamlData = loadYamlFile(yamlPath)
  const envVars = loadEnvFile(envPath)
  return mergeConfigToYaml(yamlData, envVars)
}

/**
 * 写入完整配置（拆分敏感字段到 .env，其余写 config.yaml）。
 * @returns 落盘的敏感键（供调用方判断管理员凭据/端口是否变化）
 */
export function writeFullConfig(yamlPath: string, envPath: string, config: AppConfig): EnvVars {
  const { yamlConfig, envVars } = splitConfigToEnv(config)
  saveYamlFile(yamlPath, yamlConfig)

  // 保留 .env 中非本应用管理的键（如用户手动添加的）
  const existingEnv = loadEnvFile(envPath)
  const managedKeys = new Set(SENSITIVE_FIELDS.map(f => f.envKey))
  const mergedEnv: EnvVars = { ...envVars }
  for (const [k, v] of Object.entries(existingEnv)) {
    if (!managedKeys.has(k)) {
      mergedEnv[k] = v
    }
  }
  saveEnvFile(envPath, mergedEnv)
  return mergedEnv
}

/**
 * 读取磁盘上的完整配置并按已知结构收敛。
 * 文件缺失或被手工改坏时返回 null 而不是抛异常——启动前校验与 IPC 读取共用这一条路径，
 * 两处各自实现一份迟早会漂移成「界面看到的有效配置」与「启动时用的」不一致。
 */
export function readManagedConfig(yamlPath: string, envPath: string): AppConfig | null {
  try {
    if (!fs.existsSync(yamlPath)) return null
    return trySanitize(readFullConfig(yamlPath, envPath))
  } catch {
    return null
  }
}
