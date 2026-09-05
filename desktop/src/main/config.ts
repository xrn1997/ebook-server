import yaml from 'js-yaml'
import fs from 'node:fs'
import path from 'node:path'

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

/** .env 键值对 */
export type EnvVars = Record<string, string>

/** 从 config 中拆出敏感字段的映射：configPath → envKey */
const SENSITIVE_FIELDS: Array<{ yamlPath: string[]; envKey: string }> = [
  { yamlPath: ['jwt', 'secret'], envKey: 'JWT_SECRET' },
  { yamlPath: ['smtp', 'password'], envKey: 'SMTP_PASSWORD' },
  { yamlPath: ['admin', 'password'], envKey: 'ADMIN_PASSWORD' },
  { yamlPath: ['admin', 'jwt_secret'], envKey: 'ADMIN_JWT_SECRET' },
]

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

/** 按嵌套路径从对象取值 */
function getNestedValue(obj: Record<string, unknown>, keys: string[]): unknown {
  let current: unknown = obj
  for (const key of keys) {
    if (current == null || typeof current !== 'object') return undefined
    current = (current as Record<string, unknown>)[key]
  }
  return current
}

/** 按嵌套路径向对象设值 */
function setNestedValue(obj: Record<string, unknown>, keys: string[], value: unknown): void {
  let current: Record<string, unknown> = obj
  for (let i = 0; i < keys.length - 1; i++) {
    if (!(keys[i] in current) || typeof current[keys[i]] !== 'object') {
      current[keys[i]] = {}
    }
    current = current[keys[i]] as Record<string, unknown>
  }
  current[keys[keys.length - 1]] = value
}

/** 按嵌套路径从对象删除值 */
function deleteNestedValue(obj: Record<string, unknown>, keys: string[]): void {
  let current: Record<string, unknown> = obj
  for (let i = 0; i < keys.length - 1; i++) {
    if (!(keys[i] in current)) return
    current = current[keys[i]] as Record<string, unknown>
  }
  delete current[keys[keys.length - 1]]
}

/**
 * 将 AppConfig 拆分为 YAML 部分（非敏感）和 ENV 部分（敏感）。
 * 敏感字段（密码、密钥）从 yamlConfig 中移除，放入 envVars。
 */
export function splitConfigToEnv(config: AppConfig): { yamlConfig: Record<string, unknown>; envVars: EnvVars } {
  const yamlConfig = JSON.parse(JSON.stringify(config)) as Record<string, unknown>
  const envVars: EnvVars = {}

  for (const { yamlPath, envKey } of SENSITIVE_FIELDS) {
    const value = getNestedValue(yamlConfig, yamlPath)
    if (value !== undefined && value !== '') {
      envVars[envKey] = String(value)
    }
    deleteNestedValue(yamlConfig, yamlPath)
  }

  return { yamlConfig, envVars }
}

/**
 * 将 env 变量合并回 YAML 配置对象（还原敏感字段）。
 */
export function mergeConfigToYaml(yamlConfig: Record<string, unknown>, envVars: EnvVars): AppConfig {
  const merged = JSON.parse(JSON.stringify(yamlConfig)) as Record<string, unknown>

  for (const { yamlPath, envKey } of SENSITIVE_FIELDS) {
    if (envVars[envKey] !== undefined) {
      setNestedValue(merged, yamlPath, envVars[envKey])
    }
  }

  return merged as unknown as AppConfig
}

/** 校验配置合法性，返回错误消息列表 */
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
 */
export function writeFullConfig(yamlPath: string, envPath: string, config: AppConfig): void {
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
}
