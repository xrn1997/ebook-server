# Electron 桌面管理应用实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有 Go 后端封装为 Electron 桌面应用，提供 GUI 配置管理、服务控制和现有管理功能。

**Architecture:** Electron Main Process 管理 Go sidecar 子进程生命周期，通过 IPC 暴露服务控制和配置读写 API 给 Vue 3 Renderer。Vue 前端通过 HTTP 调用 Go 后端管理 API（localhost:9091），通过 IPC 调用 Electron Main（服务控制/配置）。配置文件存放在 Electron userData 目录。

**Tech Stack:** Electron 33+, TypeScript, Vue 3, Vite 5, Vitest, js-yaml, electron-builder

**Spec:** `docs/superpowers/specs/2026-09-05-desktop-app-design.md`

---

## 文件结构总览

```
desktop/
├── package.json
├── tsconfig.json
├── tsconfig.node.json
├── electron-builder.yml
├── vitest.config.ts
├── src/
│   ├── main/
│   │   ├── index.ts              # 入口：窗口创建、托盘、生命周期
│   │   ├── sidecar.ts            # Go 子进程管理
│   │   ├── config.ts             # config.yaml / .env 读写
│   │   ├── ipc.ts                # IPC handler 注册
│   │   └── paths.ts              # 用户数据目录、二进制路径
│   ├── preload/
│   │   └── index.ts              # contextBridge 暴露 IPC API
│   └── renderer/
│       ├── index.html
│       ├── package.json          # (不用，共享根 package.json)
│       ├── vite.config.ts
│       ├── src/
│       │   ├── main.ts
│       │   ├── App.vue
│       │   ├── router.ts
│       │   ├── style.css
│       │   ├── api.ts            # 管理 API HTTP 调用
│       │   ├── electron.ts       # IPC 封装（window.electronAPI）
│       │   ├── stores/
│       │   │   └── service.ts    # 服务状态响应式 store
│       │   └── views/
│       │       ├── Overview.vue
│       │       ├── Config.vue
│       │       ├── Service.vue
│       │       ├── Users.vue
│       │       ├── Comments.vue
│       │       └── Logs.vue
│       └── tests/
│           ├── config.test.ts
│           └── Overview.test.ts
├── templates/
│   └── config.yaml               # 首次运行默认配置模板
└── resources/
    └── backend/
        └── .gitkeep              # Go 二进制编译到此目录
```

---

## Task 1: 项目脚手架

**Files:**
- Create: `desktop/package.json`
- Create: `desktop/tsconfig.json`
- Create: `desktop/tsconfig.node.json`
- Create: `desktop/vitest.config.ts`
- Create: `desktop/.gitignore`
- Modify: `.gitignore` (添加 desktop 产物)

- [ ] **Step 1: 创建 desktop/package.json**

```json
{
  "name": "ebook-server-desktop",
  "version": "0.1.0",
  "private": true,
  "main": "dist/main/index.js",
  "scripts": {
    "dev": "concurrently -k \"npm:dev:main\" \"npm:dev:renderer\"",
    "dev:main": "tsc -p tsconfig.node.json --watch",
    "dev:renderer": "vite src/renderer",
    "start": "electron .",
    "build": "npm run build:main && npm run build:renderer",
    "build:main": "tsc -p tsconfig.node.json",
    "build:renderer": "vite build src/renderer",
    "test": "vitest run",
    "test:watch": "vitest",
    "package": "npm run build && electron-builder"
  },
  "dependencies": {
    "js-yaml": "^4.1.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.0.0",
    "@types/js-yaml": "^4.0.9",
    "concurrently": "^9.0.0",
    "electron": "^33.0.0",
    "electron-builder": "^25.0.0",
    "typescript": "^5.5.0",
    "vite": "^5.4.0",
    "vitest": "^2.0.0",
    "vue": "^3.5.0",
    "vue-router": "^4.4.0",
    "vue-tsc": "^2.0.0"
  }
}
```

- [ ] **Step 2: 创建 desktop/tsconfig.node.json（Main Process + Preload）**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "Node16",
    "moduleResolution": "Node16",
    "outDir": "dist/main",
    "rootDir": "src/main",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "declaration": false,
    "sourceMap": true
  },
  "include": ["src/main/**/*.ts", "src/preload/**/*.ts"]
}
```

- [ ] **Step 3: 创建 desktop/tsconfig.json（Renderer Vue）**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "jsx": "preserve",
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "types": ["vitest/globals"]
  },
  "include": ["src/renderer/**/*.ts", "src/renderer/**/*.vue"]
}
```

- [ ] **Step 4: 创建 desktop/vitest.config.ts**

```typescript
import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    globals: true,
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
```

- [ ] **Step 5: 创建 desktop/.gitignore**

```
node_modules/
dist/
resources/backend/ebook-server*
!resources/backend/.gitkeep
```

- [ ] **Step 6: 修改根 .gitignore，添加 desktop 产物**

在根目录 `.gitignore` 末尾添加：

```
# Desktop app build
desktop/node_modules/
desktop/dist/
```

- [ ] **Step 7: 安装依赖并验证**

Run: `cd desktop && npm install`
Expected: 无报错，node_modules/ 生成

Run: `cd desktop && npx tsc --version`
Expected: `Version 5.x.x`

- [ ] **Step 8: Commit**

```bash
git add desktop/package.json desktop/tsconfig.json desktop/tsconfig.node.json desktop/vitest.config.ts desktop/.gitignore .gitignore
git commit -m "feat(desktop): 搭建 Electron 桌面应用脚手架"
```

---

## Task 2: 路径管理（paths.ts）

**Files:**
- Create: `desktop/src/main/paths.ts`
- Create: `desktop/src/main/__tests__/paths.test.ts`
- Create: `desktop/resources/backend/.gitkeep`

- [ ] **Step 1: 写 paths.ts 的失败测试**

创建 `desktop/src/main/__tests__/paths.test.ts`：

```typescript
import { describe, it, expect } from 'vitest'
import { getSidecarPath, getUserDataDir, getConfigPath, getEnvPath } from '../paths'
import path from 'node:path'

describe('paths', () => {
  it('getUserDataDir 返回 electron userData 下的 ebook-server 目录', () => {
    const dir = getUserDataDir('/fake/userData')
    expect(dir).toBe(path.join('/fake/userData', 'ebook-server'))
  })

  it('getConfigPath 返回 userData/ebook-server/config.yaml', () => {
    const p = getConfigPath('/fake/userData')
    expect(p).toBe(path.join('/fake/userData', 'ebook-server', 'config.yaml'))
  })

  it('getEnvPath 返回 userData/ebook-server/.env', () => {
    const p = getEnvPath('/fake/userData')
    expect(p).toBe(path.join('/fake/userData', 'ebook-server', '.env'))
  })

  it('getSidecarPath 在开发模式从 build/ 查找二进制', () => {
    const p = getSidecarPath('/project/root', false, 'win32')
    expect(p).toBe(path.join('/project/root', 'build', 'ebook-server.exe'))
  })

  it('getSidecarPath 在打包模式从 resources/ 查找二进制', () => {
    const p = getSidecarPath('/project/root', true, 'win32')
    expect(p).toBe(path.join('/project/root', 'resources', 'backend', 'ebook-server.exe'))
  })

  it('getSidecarPath 在 macOS 不带 .exe 后缀', () => {
    const p = getSidecarPath('/project/root', false, 'darwin')
    expect(p).toBe(path.join('/project/root', 'build', 'ebook-server'))
  })

  it('getSidecarPath 在 linux 不带 .exe 后缀', () => {
    const p = getSidecarPath('/project/root', false, 'linux')
    expect(p).toBe(path.join('/project/root', 'build', 'ebook-server'))
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd desktop && npx vitest run src/main/__tests__/paths.test.ts`
Expected: FAIL — `Cannot find module '../paths'`

- [ ] **Step 3: 实现 paths.ts**

创建 `desktop/src/main/paths.ts`：

```typescript
import path from 'node:path'

/**
 * 获取用户数据目录（存放 config.yaml、.env、ebook.db 等）。
 * @param appUserData Electron app.getPath('userData') 返回值
 */
export function getUserDataDir(appUserData: string): string {
  return path.join(appUserData, 'ebook-server')
}

/** 配置文件路径 */
export function getConfigPath(appUserData: string): string {
  return path.join(getUserDataDir(appUserData), 'config.yaml')
}

/** .env 文件路径 */
export function getEnvPath(appUserData: string): string {
  return path.join(getUserDataDir(appUserData), '.env')
}

/**
 * 获取 Go sidecar 二进制完整路径。
 * @param projectRoot 项目根目录（开发时为 repo root，打包时为 process.resourcesPath）
 * @param isPackaged Electron app.isPackaged
 * @param platform process.platform
 */
export function getSidecarPath(projectRoot: string, isPackaged: boolean, platform: string): string {
  const binaryName = platform === 'win32' ? 'ebook-server.exe' : 'ebook-server'
  if (isPackaged) {
    return path.join(projectRoot, 'resources', 'backend', binaryName)
  }
  return path.join(projectRoot, 'build', binaryName)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd desktop && npx vitest run src/main/__tests__/paths.test.ts`
Expected: 7 tests PASS

- [ ] **Step 5: 创建 resources 目录占位**

```bash
mkdir -p desktop/resources/backend
touch desktop/resources/backend/.gitkeep
```

- [ ] **Step 6: Commit**

```bash
git add desktop/src/main/paths.ts desktop/src/main/__tests__/paths.test.ts desktop/resources/backend/.gitkeep
git commit -m "feat(desktop): 实现路径管理模块（paths.ts）"
```

---

## Task 3: 配置管理（config.ts）

**Files:**
- Create: `desktop/src/main/config.ts`
- Create: `desktop/src/main/__tests__/config.test.ts`
- Create: `desktop/templates/config.yaml`

- [ ] **Step 1: 写 config.ts 的失败测试**

创建 `desktop/src/main/__tests__/config.test.ts`：

```typescript
import { describe, it, expect } from 'vitest'
import { parseEnv, serializeEnv, mergeConfigToYaml, splitConfigToEnv, validateConfig } from '../config'
import type { AppConfig } from '../config'

describe('parseEnv', () => {
  it('解析 KEY=VALUE 格式', () => {
    const result = parseEnv('SMTP_PASSWORD=abc123\nJWT_SECRET=xyz')
    expect(result).toEqual({ SMTP_PASSWORD: 'abc123', JWT_SECRET: 'xyz' })
  })

  it('忽略空行和注释行', () => {
    const result = parseEnv('# comment\n\nKEY=val\n')
    expect(result).toEqual({ KEY: 'val' })
  })

  it('处理空字符串', () => {
    const result = parseEnv('')
    expect(result).toEqual({})
  })

  it('处理带引号的值', () => {
    const result = parseEnv('KEY="hello world"')
    expect(result).toEqual({ KEY: 'hello world' })
  })
})

describe('serializeEnv', () => {
  it('序列化为 KEY=VALUE 格式', () => {
    const result = serializeEnv({ SMTP_PASSWORD: 'abc', JWT_SECRET: 'xyz' })
    expect(result).toContain('SMTP_PASSWORD=abc')
    expect(result).toContain('JWT_SECRET=xyz')
  })

  it('空对象返回空字符串', () => {
    expect(serializeEnv({})).toBe('')
  })
})

describe('splitConfigToEnv', () => {
  it('将敏感字段提取到 env，其余保留在 yaml', () => {
    const config: AppConfig = {
      server: { port: 9090, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { secret: 'my-jwt-secret', expire_min: 120 },
      smtp: { host: 'smtp.qq.com', port: 465, username: 'a@qq.com', password: 'smtp-pwd', from: 'a@qq.com', insecure: false },
      admin: { username: 'admin', password: 'admin-pwd', jwt_secret: 'admin-jwt', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    const { yamlConfig, envVars } = splitConfigToEnv(config)
    expect(yamlConfig.smtp.password).toBeUndefined()
    expect(yamlConfig.admin.password).toBeUndefined()
    expect(yamlConfig.admin.jwt_secret).toBeUndefined()
    expect(yamlConfig.jwt.secret).toBeUndefined()
    expect(envVars.SMTP_PASSWORD).toBe('smtp-pwd')
    expect(envVars.ADMIN_PASSWORD).toBe('admin-pwd')
    expect(envVars.ADMIN_JWT_SECRET).toBe('admin-jwt')
    expect(envVars.JWT_SECRET).toBe('my-jwt-secret')
  })
})

describe('mergeConfigToYaml', () => {
  it('将 env 变量合并回 config 对象', () => {
    const yamlConfig = {
      server: { port: 9090, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { expire_min: 120 },
      smtp: { host: 'smtp.qq.com', port: 465, username: 'a@qq.com', from: 'a@qq.com', insecure: false },
      admin: { username: 'admin', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    const envVars = {
      JWT_SECRET: 'jwt-s',
      SMTP_PASSWORD: 'smtp-s',
      ADMIN_PASSWORD: 'adm-p',
      ADMIN_JWT_SECRET: 'adm-j',
    }
    const merged = mergeConfigToYaml(yamlConfig, envVars)
    expect(merged.jwt.secret).toBe('jwt-s')
    expect(merged.smtp.password).toBe('smtp-s')
    expect(merged.admin.password).toBe('adm-p')
    expect(merged.admin.jwt_secret).toBe('adm-j')
  })
})

describe('validateConfig', () => {
  it('端口号超出范围返回错误', () => {
    const config: AppConfig = {
      server: { port: 99999, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { secret: 's', expire_min: 120 },
      smtp: { host: '', port: 0, username: '', password: '', from: '', insecure: false },
      admin: { username: 'a', password: 'p', jwt_secret: 'j', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    const errors = validateConfig(config)
    expect(errors).toContain('server.port 必须在 1-65535 之间')
  })

  it('JWT secret 为空返回错误', () => {
    const config: AppConfig = {
      server: { port: 9090, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { secret: '', expire_min: 120 },
      smtp: { host: '', port: 0, username: '', password: '', from: '', insecure: false },
      admin: { username: 'a', password: 'p', jwt_secret: 'j', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    const errors = validateConfig(config)
    expect(errors).toContain('jwt.secret 不能为空')
  })

  it('合法配置返回空错误列表', () => {
    const config: AppConfig = {
      server: { port: 9090, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { secret: 'valid', expire_min: 120 },
      smtp: { host: 'smtp.qq.com', port: 465, username: 'a@qq.com', password: 'p', from: 'a@qq.com', insecure: false },
      admin: { username: 'a', password: 'p', jwt_secret: 'j', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    expect(validateConfig(config)).toEqual([])
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd desktop && npx vitest run src/main/__tests__/config.test.ts`
Expected: FAIL — `Cannot find module '../config'`

- [ ] **Step 3: 实现 config.ts**

创建 `desktop/src/main/config.ts`：

```typescript
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
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd desktop && npx vitest run src/main/__tests__/config.test.ts`
Expected: 13 tests PASS

- [ ] **Step 5: 创建默认配置模板**

创建 `desktop/templates/config.yaml`：

```yaml
server:
  port: 9090
  mode: release

database:
  path: ebook.db

api_docs:
  enabled: false

jwt:
  secret: ""
  expire_min: 120

admin:
  username: admin
  password: ""
  jwt_secret: ""
  expire_min: 60
  listen_addr: 127.0.0.1
  listen_port: 9091

smtp:
  host: ""
  port: 465
  username: ""
  password: ""
  from: ""
  insecure: false

upload:
  dir: uploads
```

- [ ] **Step 6: Commit**

```bash
git add desktop/src/main/config.ts desktop/src/main/__tests__/config.test.ts desktop/templates/config.yaml
git commit -m "feat(desktop): 实现配置管理模块（config.ts）含 YAML/ENV 拆分与校验"
```

---

## Task 4: Sidecar 进程管理（sidecar.ts）

**Files:**
- Create: `desktop/src/main/sidecar.ts`
- Create: `desktop/src/main/__tests__/sidecar.test.ts`

- [ ] **Step 1: 写 sidecar.ts 的失败测试**

创建 `desktop/src/main/__tests__/sidecar.test.ts`：

```typescript
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { EventEmitter } from 'node:events'
import { SidecarManager, type SidecarStatus } from '../sidecar'

// Mock child_process
vi.mock('node:child_process', () => {
  const mockProcess = new EventEmitter() as EventEmitter & {
    pid: number
    kill: ReturnType<typeof vi.fn>
    stdout: EventEmitter
    stderr: EventEmitter
  }
  mockProcess.pid = 12345
  mockProcess.kill = vi.fn(() => {
    mockProcess.emit('exit', 0)
    return true
  })
  mockProcess.stdout = new EventEmitter()
  mockProcess.stderr = new EventEmitter()

  return {
    spawn: vi.fn(() => mockProcess),
    __mockProcess: mockProcess,
  }
})

// Mock http
vi.mock('node:http', () => ({
  get: vi.fn(),
}))

describe('SidecarManager', () => {
  let manager: SidecarManager

  beforeEach(() => {
    manager = new SidecarManager({
      binaryPath: '/fake/ebook-server',
      workDir: '/fake/workdir',
      port: 9090,
      onStatusChange: vi.fn(),
      onLog: vi.fn(),
    })
  })

  afterEach(() => {
    manager.stop()
    vi.restoreAllMocks()
  })

  it('初始状态为 stopped', () => {
    expect(manager.getStatus()).toBe('stopped')
  })

  it('getStatus 返回当前状态', () => {
    expect(manager.getStatus()).toBe('stopped')
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd desktop && npx vitest run src/main/__tests__/sidecar.test.ts`
Expected: FAIL — `Cannot find module '../sidecar'`

- [ ] **Step 3: 实现 sidecar.ts**

创建 `desktop/src/main/sidecar.ts`：

```typescript
import { spawn, type ChildProcess } from 'node:child_process'
import http from 'node:http'
import path from 'node:path'

export type SidecarStatus = 'stopped' | 'starting' | 'running' | 'stopping' | 'error'

export interface SidecarOptions {
  binaryPath: string
  workDir: string
  port: number
  onStatusChange: (status: SidecarStatus) => void
  onLog: (line: string) => void
}

/**
 * Go sidecar 子进程管理器。
 * 负责 spawn/kill/restart Go 后端二进制，并通过健康检查确认就绪。
 */
export class SidecarManager {
  private process: ChildProcess | null = null
  private status: SidecarStatus = 'stopped'
  private healthTimer: ReturnType<typeof setInterval> | null = null
  private startTimeout: ReturnType<typeof setTimeout> | null = null
  private restartCount = 0
  private readonly maxRestarts = 3
  private intentionalStop = false

  constructor(private opts: SidecarOptions) {}

  getStatus(): SidecarStatus {
    return this.status
  }

  private setStatus(s: SidecarStatus): void {
    this.status = s
    this.opts.onStatusChange(s)
  }

  /** 启动 Go 子进程 */
  start(): void {
    if (this.process) return
    this.intentionalStop = false
    this.setStatus('starting')
    this.opts.onLog(`[sidecar] Starting ${this.opts.binaryPath}`)

    const binaryName = path.basename(this.opts.binaryPath)
    this.process = spawn(this.opts.binaryPath, [], {
      cwd: this.opts.workDir,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env },
    })

    this.process.stdout?.on('data', (data: Buffer) => {
      for (const line of data.toString().split('\n').filter(Boolean)) {
        this.opts.onLog(`[go:stdout] ${line}`)
      }
    })

    this.process.stderr?.on('data', (data: Buffer) => {
      for (const line of data.toString().split('\n').filter(Boolean)) {
        this.opts.onLog(`[go:stderr] ${line}`)
      }
    })

    this.process.on('exit', (code, signal) => {
      this.opts.onLog(`[sidecar] Process exited (code=${code}, signal=${signal})`)
      this.cleanup()
      if (!this.intentionalStop) {
        this.handleCrash()
      } else {
        this.setStatus('stopped')
      }
    })

    this.startHealthCheck()
    this.startTimeout = setTimeout(() => {
      if (this.status === 'starting') {
        this.opts.onLog('[sidecar] Health check timeout (10s), marking as error')
        this.setStatus('error')
      }
    }, 10_000)
  }

  /** 停止 Go 子进程 */
  stop(): void {
    this.intentionalStop = true
    this.cleanup()
    if (this.process) {
      this.setStatus('stopping')
      this.process.kill('SIGTERM')
      // 5 秒后强杀
      const forceKillTimer = setTimeout(() => {
        if (this.process) {
          this.opts.onLog('[sidecar] Force killing process')
          this.process.kill('SIGKILL')
        }
      }, 5000)
      this.process.on('exit', () => clearTimeout(forceKillTimer), { once: true } as any)
    } else {
      this.setStatus('stopped')
    }
  }

  /** 重启：停止后启动 */
  async restart(): Promise<void> {
    this.intentionalStop = true
    this.cleanup()
    if (this.process) {
      this.setStatus('stopping')
      this.process.kill('SIGTERM')
      await new Promise<void>((resolve) => {
        if (!this.process) { resolve(); return }
        this.process.on('exit', () => resolve(), { once: true } as any)
        setTimeout(resolve, 5000)
      })
    }
    this.process = null
    this.start()
  }

  private cleanup(): void {
    if (this.healthTimer) {
      clearInterval(this.healthTimer)
      this.healthTimer = null
    }
    if (this.startTimeout) {
      clearTimeout(this.startTimeout)
      this.startTimeout = null
    }
    this.process = null
  }

  private startHealthCheck(): void {
    this.healthTimer = setInterval(() => {
      this.checkHealth()
    }, 1000)
  }

  private checkHealth(): void {
    const req = http.get(`http://localhost:${this.opts.port}/health`, (res) => {
      if (res.statusCode === 200 && this.status === 'starting') {
        this.opts.onLog('[sidecar] Health check passed, service is running')
        if (this.startTimeout) {
          clearTimeout(this.startTimeout)
          this.startTimeout = null
        }
        this.setStatus('running')
        this.restartCount = 0
      }
    })
    req.on('error', () => {
      // 健康检查失败，继续等待
    })
    req.setTimeout(2000, () => req.destroy())
  }

  private handleCrash(): void {
    if (this.restartCount < this.maxRestarts) {
      this.restartCount++
      const delay = this.restartCount * 2000
      this.opts.onLog(`[sidecar] Auto-restart attempt ${this.restartCount}/${this.maxRestarts} in ${delay}ms`)
      this.setStatus('error')
      setTimeout(() => {
        this.process = null
        this.start()
      }, delay)
    } else {
      this.opts.onLog('[sidecar] Max restart attempts reached, marking as error')
      this.setStatus('error')
    }
  }
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd desktop && npx vitest run src/main/__tests__/sidecar.test.ts`
Expected: 2 tests PASS

- [ ] **Step 5: Commit**

```bash
git add desktop/src/main/sidecar.ts desktop/src/main/__tests__/sidecar.test.ts
git commit -m "feat(desktop): 实现 Go sidecar 进程管理（sidecar.ts）"
```

---

## Task 5: IPC 通道（ipc.ts）

**Files:**
- Create: `desktop/src/main/ipc.ts`

- [ ] **Step 1: 实现 ipc.ts**

创建 `desktop/src/main/ipc.ts`：

```typescript
import { ipcMain, type BrowserWindow } from 'electron'
import fs from 'node:fs'
import {
  readFullConfig,
  writeFullConfig,
  validateConfig,
  type AppConfig,
} from './config'
import { SidecarManager, type SidecarStatus } from './sidecar'
import { getConfigPath, getEnvPath } from './paths'

/** IPC 通道名常量 */
export const Channels = {
  GET_SERVICE_STATUS: 'get-service-status',
  RESTART_SERVICE: 'restart-service',
  STOP_SERVICE: 'stop-service',
  START_SERVICE: 'start-service',
  GET_CONFIG: 'get-config',
  SAVE_CONFIG: 'save-config',
  GET_LOGS: 'get-logs',
  STATUS_CHANGED: 'status-changed',
  LOG_LINE: 'log-line',
} as const

/** 日志缓冲（最近 500 行） */
const logBuffer: string[] = []
const MAX_LOG_LINES = 500

function addLog(line: string): void {
  logBuffer.push(line)
  if (logBuffer.length > MAX_LOG_LINES) logBuffer.shift()
}

/** 注册所有 IPC handler */
export function registerIpcHandlers(
  sidecar: SidecarManager,
  userDataPath: string,
  getMainWindow: () => BrowserWindow | null,
): void {
  const configPath = getConfigPath(userDataPath)
  const envPath = getEnvPath(userDataPath)

  // 服务状态查询
  ipcMain.handle(Channels.GET_SERVICE_STATUS, () => {
    return { status: sidecar.getStatus() }
  })

  // 服务控制
  ipcMain.handle(Channels.START_SERVICE, () => {
    sidecar.start()
    return { ok: true }
  })

  ipcMain.handle(Channels.STOP_SERVICE, () => {
    sidecar.stop()
    return { ok: true }
  })

  ipcMain.handle(Channels.RESTART_SERVICE, async () => {
    await sidecar.restart()
    return { ok: true }
  })

  // 配置读取
  ipcMain.handle(Channels.GET_CONFIG, () => {
    if (!fs.existsSync(configPath)) {
      return { error: 'config.yaml not found' }
    }
    const config = readFullConfig(configPath, envPath)
    return { config }
  })

  // 配置保存
  ipcMain.handle(Channels.SAVE_CONFIG, (_event, config: AppConfig) => {
    const errors = validateConfig(config)
    if (errors.length > 0) {
      return { errors }
    }
    writeFullConfig(configPath, envPath, config)
    return { ok: true }
  })

  // 获取日志缓冲
  ipcMain.handle(Channels.GET_LOGS, () => {
    return { lines: [...logBuffer] }
  })

  // 转发 sidecar 状态变化到 renderer
  sidecar.opts.onStatusChange = (status: SidecarStatus) => {
    const win = getMainWindow()
    win?.webContents.send(Channels.STATUS_CHANGED, { status })
  }

  // 转发日志到 renderer
  sidecar.opts.onLog = (line: string) => {
    addLog(line)
    const win = getMainWindow()
    win?.webContents.send(Channels.LOG_LINE, { line })
  }
}

/** 注销所有 IPC handler（应用退出时调用） */
export function unregisterIpcHandlers(): void {
  for (const channel of Object.values(Channels)) {
    ipcMain.removeHandler(channel)
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add desktop/src/main/ipc.ts
git commit -m "feat(desktop): 实现 IPC 通道注册（ipc.ts）"
```

---

## Task 6: Preload 脚本

**Files:**
- Create: `desktop/src/preload/index.ts`

- [ ] **Step 1: 实现 preload/index.ts**

创建 `desktop/src/preload/index.ts`：

```typescript
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
```

- [ ] **Step 2: Commit**

```bash
git add desktop/src/preload/index.ts
git commit -m "feat(desktop): 实现 preload 脚本（contextBridge 暴露 IPC API）"
```

---

## Task 7: Electron 主入口（index.ts）

**Files:**
- Create: `desktop/src/main/index.ts`

- [ ] **Step 1: 实现 main/index.ts**

创建 `desktop/src/main/index.ts`：

```typescript
import { app, BrowserWindow, Tray, Menu, nativeImage } from 'electron'
import path from 'node:path'
import fs from 'node:fs'
import { SidecarManager } from './sidecar'
import { registerIpcHandlers, unregisterIpcHandlers } from './ipc'
import { getSidecarPath, getUserDataDir, getConfigPath, getEnvPath } from './paths'

let mainWindow: BrowserWindow | null = null
let tray: Tray | null = null
let sidecar: SidecarManager

const isDev = !app.isPackaged

function createWindow(): void {
  mainWindow = new BrowserWindow({
    width: 1100,
    height: 750,
    minWidth: 900,
    minHeight: 600,
    title: 'ebook-server',
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'index.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  if (isDev) {
    mainWindow.loadURL('http://localhost:5173')
  } else {
    mainWindow.loadFile(path.join(__dirname, '..', 'renderer', 'index.html'))
  }

  mainWindow.on('closed', () => {
    mainWindow = null
  })
}

function createTray(): void {
  // 使用 16x16 空图标作为占位（实际项目应提供 .ico/.png 图标）
  const icon = nativeImage.createEmpty()
  tray = new Tray(icon)
  tray.setToolTip('ebook-server')

  const contextMenu = Menu.buildFromTemplate([
    { label: '显示主窗口', click: () => mainWindow?.show() },
    { type: 'separator' },
    {
      label: '重启服务',
      click: () => sidecar.restart(),
    },
    { type: 'separator' },
    { label: '退出', click: () => app.quit() },
  ])
  tray.setContextMenu(contextMenu)
  tray.on('click', () => mainWindow?.show())
}

function ensureUserData(): void {
  const userDataDir = getUserDataDir(app.getPath('userData'))
  fs.mkdirSync(userDataDir, { recursive: true })

  const configPath = getConfigPath(app.getPath('userData'))
  if (!fs.existsSync(configPath)) {
    // 从模板复制默认配置
    const templatePath = isDev
      ? path.join(app.getAppPath(), 'templates', 'config.yaml')
      : path.join(process.resourcesPath, 'templates', 'config.yaml')
    if (fs.existsSync(templatePath)) {
      fs.copyFileSync(templatePath, configPath)
    }
  }

  const envPath = getEnvPath(app.getPath('userData'))
  if (!fs.existsSync(envPath)) {
    fs.writeFileSync(envPath, '', 'utf-8')
  }
}

function startSidecar(): void {
  const projectRoot = isDev ? path.join(app.getAppPath(), '..') : process.resourcesPath
  const binaryPath = getSidecarPath(projectRoot, app.isPackaged, process.platform)
  const workDir = getUserDataDir(app.getPath('userData'))

  sidecar = new SidecarManager({
    binaryPath,
    workDir,
    port: 9090,
    onStatusChange: () => {}, // ipc.ts 中会覆盖
    onLog: () => {},          // ipc.ts 中会覆盖
  })

  registerIpcHandlers(sidecar, app.getPath('userData'), () => mainWindow)
  sidecar.start()
}

app.whenReady().then(() => {
  ensureUserData()
  createWindow()
  createTray()
  startSidecar()
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow()
  }
})

app.on('before-quit', () => {
  sidecar?.stop()
  unregisterIpcHandlers()
})
```

- [ ] **Step 2: Commit**

```bash
git add desktop/src/main/index.ts
git commit -m "feat(desktop): 实现 Electron 主入口（窗口、托盘、sidecar 启动）"
```

---

## Task 8: Vue Renderer 骨架

**Files:**
- Create: `desktop/src/renderer/index.html`
- Create: `desktop/src/renderer/vite.config.ts`
- Create: `desktop/src/renderer/src/main.ts`
- Create: `desktop/src/renderer/src/App.vue`
- Create: `desktop/src/renderer/src/router.ts`
- Create: `desktop/src/renderer/src/style.css`
- Create: `desktop/src/renderer/src/electron.ts`
- Create: `desktop/src/renderer/src/api.ts`
- Create: `desktop/src/renderer/src/stores/service.ts`
- Create: `desktop/src/renderer/env.d.ts`

- [ ] **Step 1: 创建 renderer/index.html**

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>ebook-server</title>
</head>
<body>
  <div id="app"></div>
  <script type="module" src="/src/main.ts"></script>
</body>
</html>
```

- [ ] **Step 2: 创建 renderer/vite.config.ts**

```typescript
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  root: 'src/renderer',
  base: './',
  build: {
    outDir: '../../dist/renderer',
    emptyDir: true,
  },
  server: {
    port: 5173,
  },
})
```

- [ ] **Step 3: 创建 env.d.ts（TypeScript 类型声明）**

创建 `desktop/src/renderer/env.d.ts`：

```typescript
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
```

- [ ] **Step 4: 创建 electron.ts（IPC 封装）**

```typescript
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
```

- [ ] **Step 5: 创建 api.ts（管理 API HTTP 调用）**

```typescript
/**
 * 管理后台 API 调用（通过 HTTP 访问 Go sidecar 的 /admin/api/* 端点）。
 * 与现有 frontend/src/api.js 功能一致，但 base URL 指向 sidecar 的 9091 端口。
 */

const ADMIN_BASE = 'http://localhost:9091/admin/api'

let adminToken = localStorage.getItem('admin_token') || ''

export function setToken(t: string): void {
  adminToken = t
  if (t) localStorage.setItem('admin_token', t)
  else localStorage.removeItem('admin_token')
}

export function getToken(): string {
  return adminToken
}

async function request(path: string, { method = 'GET', body }: { method?: string; body?: unknown } = {}) {
  const res = await fetch(ADMIN_BASE + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(adminToken ? { Authorization: 'Bearer ' + adminToken } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  return res.json()
}

export async function login(username: string, password: string) {
  return request('/login', { method: 'POST', body: { username, password } })
}

export async function fetchStats() {
  return request('/stats')
}

export async function fetchUsers(page = 1, pageSize = 20) {
  return request(`/users?page=${page}&page_size=${pageSize}`)
}

export async function fetchComments(page = 1, pageSize = 20) {
  return request(`/comments?page=${page}&page_size=${pageSize}`)
}

export async function fetchLogs(page = 1, pageSize = 20) {
  return request(`/logs?page=${page}&page_size=${pageSize}`)
}
```

- [ ] **Step 6: 创建 stores/service.ts（响应式服务状态）**

```typescript
import { ref } from 'vue'
import * as ipc from '../electron'

export type ServiceStatus = 'stopped' | 'starting' | 'running' | 'stopping' | 'error' | 'unknown'

export const serviceStatus = ref<ServiceStatus>('unknown')
export const serviceLogs = ref<string[]>([])

/** 初始化服务状态监听（在 App.vue onMounted 中调用） */
export function initServiceListener(): void {
  // 获取初始状态
  ipc.getServiceStatus().then((status) => {
    serviceStatus.value = status as ServiceStatus
  })

  // 监听状态变化
  const unsubStatus = ipc.onStatusChange((status) => {
    serviceStatus.value = status as ServiceStatus
  })

  // 监听日志
  const unsubLog = ipc.onLogLine((line) => {
    serviceLogs.value.push(line)
    if (serviceLogs.value.length > 500) {
      serviceLogs.value = serviceLogs.value.slice(-500)
    }
  })

  // 返回清理函数
  return () => {
    unsubStatus?.()
    unsubLog?.()
  }
}
```

- [ ] **Step 7: 创建 router.ts**

```typescript
import { createRouter, createWebHashHistory } from 'vue-router'
import Overview from './views/Overview.vue'
import Config from './views/Config.vue'
import Service from './views/Service.vue'
import Users from './views/Users.vue'
import Comments from './views/Comments.vue'
import Logs from './views/Logs.vue'

const routes = [
  { path: '/', name: 'overview', component: Overview },
  { path: '/config', name: 'config', component: Config },
  { path: '/service', name: 'service', component: Service },
  { path: '/users', name: 'users', component: Users },
  { path: '/comments', name: 'comments', component: Comments },
  { path: '/logs', name: 'logs', component: Logs },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

export default router
```

- [ ] **Step 8: 创建 style.css**

```css
* { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: system-ui, "PingFang SC", "Microsoft YaHei", sans-serif;
  background: #f1f5f9;
  color: #0f172a;
}

a { color: inherit; text-decoration: none; }
button { cursor: pointer; }

.layout { display: flex; min-height: 100vh; }

.sidebar {
  width: 200px;
  background: #0f172a;
  color: #e2e8f0;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sidebar .brand {
  font-weight: 700;
  font-size: 18px;
  padding: 8px 12px 16px;
}

.sidebar nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.sidebar nav a {
  padding: 10px 12px;
  border-radius: 8px;
  font-size: 14px;
  transition: background 0.15s;
}

.sidebar nav a:hover { background: #1e293b; }
.sidebar nav a.active { background: #334155; font-weight: 600; }

.sidebar .status-bar {
  margin-top: auto;
  padding: 10px 12px;
  font-size: 12px;
  color: #94a3b8;
  display: flex;
  align-items: center;
  gap: 6px;
}

.sidebar .status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #94a3b8;
}

.sidebar .status-dot.running { background: #22c55e; }
.sidebar .status-dot.error { background: #ef4444; }
.sidebar .status-dot.starting, .sidebar .status-dot.stopping { background: #f59e0b; }

.content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

h2 { font-size: 20px; margin-bottom: 8px; }
.sub { color: #64748b; font-size: 14px; margin-bottom: 16px; }

.card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 20px;
  margin-bottom: 16px;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
}

.stat-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 20px;
}

.stat-card .num { font-size: 28px; font-weight: 700; }
.stat-card .label { color: #64748b; font-size: 13px; margin-top: 4px; }

.btn {
  padding: 8px 16px;
  border: none;
  border-radius: 8px;
  font-size: 14px;
  transition: opacity 0.15s;
}

.btn:hover { opacity: 0.85; }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-primary { background: #0f172a; color: #fff; }
.btn-danger { background: #dc2626; color: #fff; }
.btn-success { background: #16a34a; color: #fff; }
.btn-secondary { background: #e2e8f0; color: #0f172a; }

.form-group {
  margin-bottom: 16px;
}

.form-group label {
  display: block;
  font-size: 13px;
  color: #475569;
  margin-bottom: 4px;
  font-weight: 500;
}

.form-group input, .form-group select {
  width: 100%;
  padding: 8px 12px;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  font-size: 14px;
}

.form-group input:focus, .form-group select:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15);
}

.tabs {
  display: flex;
  gap: 0;
  border-bottom: 1px solid #e2e8f0;
  margin-bottom: 20px;
}

.tab {
  padding: 10px 20px;
  border: none;
  background: none;
  font-size: 14px;
  color: #64748b;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
}

.tab:hover { color: #0f172a; }
.tab.active { color: #0f172a; border-bottom-color: #0f172a; font-weight: 600; }

table {
  width: 100%;
  border-collapse: collapse;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  overflow: hidden;
}

th, td {
  text-align: left;
  padding: 10px 14px;
  border-bottom: 1px solid #e2e8f0;
  font-size: 14px;
}

th { background: #f8fafc; color: #475569; }

.err { color: #dc2626; }
.ok { color: #16a34a; }

.badge {
  padding: 1px 8px;
  border-radius: 9999px;
  font-size: 12px;
  font-weight: 600;
}

.badge.ok { background: #ecfdf5; color: #047857; }
.badge.err { background: #fef2f2; color: #b91c1c; }

.log-viewer {
  background: #0f172a;
  color: #e2e8f0;
  font-family: "Cascadia Code", "Fira Code", monospace;
  font-size: 12px;
  padding: 16px;
  border-radius: 12px;
  max-height: 400px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.6;
}

.alert {
  padding: 12px 16px;
  border-radius: 8px;
  font-size: 14px;
  margin-bottom: 16px;
}

.alert-warning { background: #fef3c7; color: #92400e; }
.alert-error { background: #fef2f2; color: #991b1b; }
.alert-success { background: #ecfdf5; color: #065f46; }
```

- [ ] **Step 9: 创建 main.ts**

```typescript
import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import './style.css'

createApp(App).use(router).mount('#app')
```

- [ ] **Step 10: 创建 App.vue**

```vue
<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { serviceStatus, initServiceListener } from './stores/service'

const route = useRoute()

const nav = [
  { to: '/', label: '概览' },
  { to: '/config', label: '配置' },
  { to: '/service', label: '服务' },
  { to: '/users', label: '用户' },
  { to: '/comments', label: '评论' },
  { to: '/logs', label: '日志' },
]

let cleanup: (() => void) | undefined

onMounted(() => {
  cleanup = initServiceListener()
})

onUnmounted(() => {
  cleanup?.()
})

function statusLabel(s: string): string {
  const map: Record<string, string> = {
    running: '运行中',
    stopped: '已停止',
    starting: '启动中',
    stopping: '停止中',
    error: '异常',
    unknown: '未知',
  }
  return map[s] || s
}
</script>

<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="brand">ebook-server</div>
      <nav>
        <router-link v-for="n in nav" :key="n.to" :to="n.to" active-class="active">{{ n.label }}</router-link>
      </nav>
      <div class="status-bar">
        <span class="status-dot" :class="serviceStatus"></span>
        {{ statusLabel(serviceStatus) }}
      </div>
    </aside>
    <main class="content">
      <router-view />
    </main>
  </div>
</template>
```

- [ ] **Step 11: 创建占位 views（后续 Task 填充）**

为每个 view 创建最小占位组件，确保路由不报错：

`desktop/src/renderer/src/views/Overview.vue`：
```vue
<template><div><h2>概览</h2><p class="sub">加载中...</p></div></template>
```

`desktop/src/renderer/src/views/Config.vue`：
```vue
<template><div><h2>配置</h2><p class="sub">加载中...</p></div></template>
```

`desktop/src/renderer/src/views/Service.vue`：
```vue
<template><div><h2>服务</h2><p class="sub">加载中...</p></div></template>
```

`desktop/src/renderer/src/views/Users.vue`：
```vue
<template><div><h2>用户管理</h2><p class="sub">加载中...</p></div></template>
```

`desktop/src/renderer/src/views/Comments.vue`：
```vue
<template><div><h2>评论管理</h2><p class="sub">加载中...</p></div></template>
```

`desktop/src/renderer/src/views/Logs.vue`：
```vue
<template><div><h2>操作日志</h2><p class="sub">加载中...</p></div></template>
```

- [ ] **Step 12: 验证 dev server 启动**

Run: `cd desktop && npm run dev:renderer`
Expected: Vite dev server 在 http://localhost:5173 启动，打开后看到侧边栏导航和占位内容

- [ ] **Step 13: Commit**

```bash
git add desktop/src/renderer/
git commit -m "feat(desktop): 搭建 Vue 3 Renderer 骨架（布局、路由、IPC 封装）"
```

---

## Task 9: 概览页（Overview.vue）

**Files:**
- Modify: `desktop/src/renderer/src/views/Overview.vue`

- [ ] **Step 1: 实现 Overview.vue**

替换 `desktop/src/renderer/src/views/Overview.vue`：

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { serviceStatus } from '../stores/service'
import { fetchStats } from '../api'
import * as ipc from '../electron'

const stats = ref<{ users: number; comments: number } | null>(null)
const statsError = ref('')

onMounted(async () => {
  try {
    const resp = await fetchStats()
    if (resp.code === '00000') {
      stats.value = resp.data
    } else {
      statsError.value = '无法连接后端服务'
    }
  } catch {
    statsError.value = '后端服务未启动'
  }
})

async function handleRestart() {
  await ipc.restartService()
}

async function handleStop() {
  await ipc.stopService()
}
</script>

<template>
  <div>
    <h2>概览</h2>
    <p class="sub">服务状态与基础数据统计</p>

    <div class="grid" style="margin-bottom: 24px;">
      <div class="stat-card">
        <div class="num" :style="{ color: serviceStatus === 'running' ? '#16a34a' : serviceStatus === 'error' ? '#dc2626' : '#64748b' }">
          {{ serviceStatus === 'running' ? '●' : serviceStatus === 'error' ? '✕' : '○' }}
        </div>
        <div class="label">服务状态：{{ serviceStatus }}</div>
      </div>
      <div class="stat-card">
        <div class="num">{{ stats?.users ?? '-' }}</div>
        <div class="label">注册用户</div>
      </div>
      <div class="stat-card">
        <div class="num">{{ stats?.comments ?? '-' }}</div>
        <div class="label">评论总数</div>
      </div>
    </div>

    <div class="card">
      <h3 style="font-size: 16px; margin-bottom: 12px;">快捷操作</h3>
      <div style="display: flex; gap: 8px;">
        <button class="btn btn-primary" @click="handleRestart" :disabled="serviceStatus === 'starting'">
          重启服务
        </button>
        <button class="btn btn-danger" @click="handleStop" :disabled="serviceStatus === 'stopped' || serviceStatus === 'stopping'">
          停止服务
        </button>
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Commit**

```bash
git add desktop/src/renderer/src/views/Overview.vue
git commit -m "feat(desktop): 实现概览页（服务状态、统计、快捷操作）"
```

---

## Task 10: 配置页（Config.vue）

**Files:**
- Modify: `desktop/src/renderer/src/views/Config.vue`

- [ ] **Step 1: 实现 Config.vue**

替换 `desktop/src/renderer/src/views/Config.vue`：

```vue
<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import * as ipc from '../electron'

const activeTab = ref('smtp')
const config = ref<any>(null)
const loading = ref(true)
const saving = ref(false)
const saveResult = ref<{ ok?: boolean; errors?: string[]; needRestart?: boolean } | null>(null)

const tabs = [
  { key: 'smtp', label: '邮件服务' },
  { key: 'security', label: '安全密钥' },
  { key: 'admin', label: '管理员' },
  { key: 'server', label: '服务设置' },
]

onMounted(async () => {
  config.value = await ipc.getConfig()
  loading.value = false
})

const smtpWarning = computed(() => {
  if (!config.value) return ''
  return !config.value.smtp?.password ? 'SMTP 密码未设置，邮件功能将不可用' : ''
})

async function save() {
  saving.value = true
  saveResult.value = null
  const result = await ipc.saveConfig(config.value)
  saving.value = false
  if (result.errors?.length) {
    saveResult.value = { errors: result.errors }
  } else {
    saveResult.value = { ok: true, needRestart: true }
  }
}

async function restartNow() {
  await ipc.restartService()
  saveResult.value = null
}
</script>

<template>
  <div>
    <h2>配置</h2>
    <p class="sub">修改配置后需重启服务生效</p>

    <div v-if="loading" class="sub">加载中...</div>

    <div v-else-if="config">
      <div class="tabs">
        <button v-for="t in tabs" :key="t.key" class="tab" :class="{ active: activeTab === t.key }" @click="activeTab = t.key">
          {{ t.label }}
        </button>
      </div>

      <div v-if="saveResult?.errors?.length" class="alert alert-error">
        <div v-for="e in saveResult.errors" :key="e">{{ e }}</div>
      </div>
      <div v-if="saveResult?.ok" class="alert alert-success">
        配置已保存。
        <a href="#" @click.prevent="restartNow" style="text-decoration: underline;">立即重启服务</a>
      </div>

      <!-- 邮件服务 Tab -->
      <div v-if="activeTab === 'smtp'" class="card">
        <div v-if="smtpWarning" class="alert alert-warning">{{ smtpWarning }}</div>
        <div class="form-group">
          <label>SMTP 主机</label>
          <input v-model="config.smtp.host" placeholder="smtp.qq.com" />
        </div>
        <div class="form-group">
          <label>端口</label>
          <input v-model.number="config.smtp.port" type="number" placeholder="465" />
        </div>
        <div class="form-group">
          <label>发信账号</label>
          <input v-model="config.smtp.username" placeholder="no-reply@example.com" />
        </div>
        <div class="form-group">
          <label>授权码/密码</label>
          <input v-model="config.smtp.password" type="password" placeholder="SMTP 授权码" />
        </div>
        <div class="form-group">
          <label>发件人</label>
          <input v-model="config.smtp.from" placeholder="no-reply@example.com" />
        </div>
        <div class="form-group">
          <label>
            <input type="checkbox" v-model="config.smtp.insecure" /> 关闭 TLS 校验（仅开发环境）
          </label>
        </div>
      </div>

      <!-- 安全密钥 Tab -->
      <div v-if="activeTab === 'security'" class="card">
        <div class="form-group">
          <label>JWT Secret</label>
          <input v-model="config.jwt.secret" placeholder="随机字符串" />
        </div>
        <div class="form-group">
          <label>JWT 过期时间（分钟）</label>
          <input v-model.number="config.jwt.expire_min" type="number" />
        </div>
        <div class="form-group">
          <label>Admin JWT Secret</label>
          <input v-model="config.admin.jwt_secret" placeholder="管理端随机字符串" />
        </div>
      </div>

      <!-- 管理员 Tab -->
      <div v-if="activeTab === 'admin'" class="card">
        <div class="form-group">
          <label>管理员用户名</label>
          <input v-model="config.admin.username" />
        </div>
        <div class="form-group">
          <label>管理员密码</label>
          <input v-model="config.admin.password" type="password" />
        </div>
      </div>

      <!-- 服务设置 Tab -->
      <div v-if="activeTab === 'server'" class="card">
        <div class="form-group">
          <label>公开 API 端口</label>
          <input v-model.number="config.server.port" type="number" />
        </div>
        <div class="form-group">
          <label>运行模式</label>
          <select v-model="config.server.mode">
            <option value="debug">debug</option>
            <option value="release">release</option>
          </select>
        </div>
        <div class="form-group">
          <label>数据库路径</label>
          <input v-model="config.database.path" placeholder="ebook.db" />
        </div>
        <div class="form-group">
          <label>上传目录</label>
          <input v-model="config.upload.dir" placeholder="uploads" />
        </div>
        <div class="form-group">
          <label>管理后台监听地址</label>
          <input v-model="config.admin.listen_addr" placeholder="127.0.0.1" />
        </div>
        <div class="form-group">
          <label>管理后台端口</label>
          <input v-model.number="config.admin.listen_port" type="number" />
        </div>
        <div class="form-group">
          <label>
            <input type="checkbox" v-model="config.api_docs.enabled" /> 公开 API 端口提供 Swagger 文档
          </label>
        </div>
      </div>

      <div style="margin-top: 16px; display: flex; gap: 8px;">
        <button class="btn btn-primary" @click="save" :disabled="saving">
          {{ saving ? '保存中...' : '保存配置' }}
        </button>
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Commit**

```bash
git add desktop/src/renderer/src/views/Config.vue
git commit -m "feat(desktop): 实现配置页（SMTP/密钥/管理员/服务设置 Tab 表单）"
```

---

## Task 11: 服务页（Service.vue）

**Files:**
- Modify: `desktop/src/renderer/src/views/Service.vue`

- [ ] **Step 1: 实现 Service.vue**

替换 `desktop/src/renderer/src/views/Service.vue`：

```vue
<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { serviceStatus, serviceLogs } from '../stores/service'
import * as ipc from '../electron'

const logContainer = ref<HTMLElement | null>(null)
let cleanup: (() => void) | undefined

onMounted(async () => {
  // 加载历史日志
  const lines = await ipc.getLogs()
  serviceLogs.value = lines
  await nextTick()
  scrollToEnd()
})

onUnmounted(() => {
  cleanup?.()
})

function scrollToEnd() {
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

async function handleStart() {
  await ipc.startService()
}

async function handleStop() {
  await ipc.stopService()
}

async function handleRestart() {
  await ipc.restartService()
}
</script>

<template>
  <div>
    <h2>服务</h2>
    <p class="sub">管理 Go 后端服务进程</p>

    <div class="card">
      <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 16px;">
        <span class="status-dot" :class="serviceStatus" style="width: 12px; height: 12px;"></span>
        <strong>当前状态：{{ serviceStatus }}</strong>
      </div>
      <div style="display: flex; gap: 8px;">
        <button class="btn btn-success" @click="handleStart" :disabled="serviceStatus === 'running' || serviceStatus === 'starting'">
          启动
        </button>
        <button class="btn btn-danger" @click="handleStop" :disabled="serviceStatus === 'stopped' || serviceStatus === 'stopping'">
          停止
        </button>
        <button class="btn btn-primary" @click="handleRestart" :disabled="serviceStatus === 'starting'">
          重启
        </button>
      </div>
    </div>

    <div class="card">
      <h3 style="font-size: 16px; margin-bottom: 12px;">实时日志</h3>
      <div ref="logContainer" class="log-viewer">{{ serviceLogs.join('\n') || '（暂无日志）' }}</div>
    </div>
  </div>
</template>

<style scoped>
.status-dot {
  display: inline-block;
  border-radius: 50%;
  background: #94a3b8;
}
.status-dot.running { background: #22c55e; }
.status-dot.error { background: #ef4444; }
.status-dot.starting, .status-dot.stopping { background: #f59e0b; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add desktop/src/renderer/src/views/Service.vue
git commit -m "feat(desktop): 实现服务页（启停控制 + 实时日志流）"
```

---

## Task 12: 迁移现有管理视图

**Files:**
- Modify: `desktop/src/renderer/src/views/Users.vue`
- Modify: `desktop/src/renderer/src/views/Comments.vue`
- Modify: `desktop/src/renderer/src/views/Logs.vue`

- [ ] **Step 1: 实现 Users.vue**

替换 `desktop/src/renderer/src/views/Users.vue`：

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fetchUsers } from '../api'

const rows = ref<any[]>([])
const error = ref('')

onMounted(async () => {
  try {
    const resp = await fetchUsers()
    if (resp.code === '00000') rows.value = resp.data.list
    else error.value = resp.error || '加载失败'
  } catch {
    error.value = '无法连接后端服务'
  }
})
</script>

<template>
  <div>
    <h2>用户管理</h2>
    <p class="sub">账号列表</p>
    <p v-if="error" class="err">{{ error }}</p>
    <table v-else>
      <thead>
        <tr><th>UID</th><th>邮箱</th><th>展示名</th><th>昵称</th><th>注册时间</th></tr>
      </thead>
      <tbody>
        <tr v-for="u in rows" :key="u.uid">
          <td>{{ u.uid }}</td>
          <td>{{ u.email }}</td>
          <td>{{ u.username }}</td>
          <td>{{ u.nickname }}</td>
          <td>{{ u.created_at }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
```

- [ ] **Step 2: 实现 Comments.vue**

替换 `desktop/src/renderer/src/views/Comments.vue`：

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fetchComments } from '../api'

const rows = ref<any[]>([])
const error = ref('')

onMounted(async () => {
  try {
    const resp = await fetchComments()
    if (resp.code === '00000') rows.value = resp.data.list
    else error.value = resp.error || '加载失败'
  } catch {
    error.value = '无法连接后端服务'
  }
})
</script>

<template>
  <div>
    <h2>评论管理</h2>
    <p class="sub">评论列表</p>
    <p v-if="error" class="err">{{ error }}</p>
    <table v-else>
      <thead>
        <tr><th>ID</th><th>作者</th><th>内容</th><th>时间</th></tr>
      </thead>
      <tbody>
        <tr v-for="c in rows" :key="c.id">
          <td>{{ c.id }}</td>
          <td>{{ c.user?.username || c.user_id }}</td>
          <td>{{ c.content }}</td>
          <td>{{ c.created_at }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
```

- [ ] **Step 3: 实现 Logs.vue**

替换 `desktop/src/renderer/src/views/Logs.vue`：

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fetchLogs } from '../api'

const rows = ref<any[]>([])
const error = ref('')
const openId = ref<number | null>(null)

onMounted(async () => {
  try {
    const resp = await fetchLogs()
    if (resp.code === '00000') rows.value = resp.data.list
    else error.value = resp.error || '加载失败'
  } catch {
    error.value = '无法连接后端服务'
  }
})

function toggle(id: number) {
  openId.value = openId.value === id ? null : id
}
</script>

<template>
  <div>
    <h2>操作日志</h2>
    <p class="sub">后台审计视图：点击行可展开详情</p>
    <p v-if="error" class="err">{{ error }}</p>
    <table v-else>
      <thead>
        <tr>
          <th>时间</th>
          <th>方法</th>
          <th>路径</th>
          <th>IP</th>
          <th>业务码</th>
        </tr>
      </thead>
      <tbody>
        <template v-for="l in rows" :key="l.id">
          <tr style="cursor: pointer;" @click="toggle(l.id)" @mouseover="($event.currentTarget as HTMLElement).style.background='#f8fafc'" @mouseout="($event.currentTarget as HTMLElement).style.background=''">
            <td>{{ l.created_at }}</td>
            <td><code>{{ l.method }}</code></td>
            <td style="max-width: 300px; overflow: hidden; text-overflow: ellipsis;">{{ l.path }}</td>
            <td>{{ l.ip }}</td>
            <td>
              <span class="badge" :class="l.error_code && l.error_code !== '00000' ? 'err' : 'ok'">
                {{ l.error_code || '—' }}
              </span>
            </td>
          </tr>
          <tr v-if="openId === l.id">
            <td colspan="5" style="background: #f8fafc;">
              <dl style="display: grid; grid-template-columns: 110px 1fr; gap: 6px 12px; margin: 0;">
                <div><dt style="color: #64748b;">ID</dt><dd style="margin: 0;">{{ l.id }}</dd></div>
                <div><dt style="color: #64748b;">用户</dt><dd style="margin: 0;">{{ l.user_id || l.username || '-' }}</dd></div>
                <div><dt style="color: #64748b;">状态码</dt><dd style="margin: 0;">{{ l.response_code }}</dd></div>
                <div><dt style="color: #64748b;">业务码</dt><dd style="margin: 0;">{{ l.error_code || '—' }}</dd></div>
                <div><dt style="color: #64748b;">业务文案</dt><dd style="margin: 0;">{{ l.error_message || '—' }}</dd></div>
                <div><dt style="color: #64748b;">User-Agent</dt><dd style="margin: 0; word-break: break-all;">{{ l.user_agent || '—' }}</dd></div>
              </dl>
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>
```

- [ ] **Step 4: Commit**

```bash
git add desktop/src/renderer/src/views/Users.vue desktop/src/renderer/src/views/Comments.vue desktop/src/renderer/src/views/Logs.vue
git commit -m "feat(desktop): 迁移现有管理视图（用户/评论/日志）"
```

---

## Task 13: 构建与打包

**Files:**
- Create: `desktop/electron-builder.yml`
- Modify: `Makefile`（添加 desktop 构建目标）

- [ ] **Step 1: 创建 electron-builder.yml**

```yaml
appId: com.ebook-server.desktop
productName: ebook-server

directories:
  output: release
  buildResources: resources

files:
  - dist/**/*
  - templates/**/*

extraResources:
  - from: resources/backend
    to: backend
    filter:
      - ebook-server*
      - '!**/.gitkeep'
  - from: templates
    to: templates

win:
  target: nsis
  icon: resources/icon.ico

nsis:
  oneClick: false
  allowToChangeInstallationDirectory: true

mac:
  target: dmg
  icon: resources/icon.icns

linux:
  target:
    - AppImage
    - deb
  icon: resources/icon.png
```

- [ ] **Step 2: 在 Makefile 末尾添加 desktop 构建目标**

在根目录 `Makefile` 末尾追加：

```makefile
# ── Desktop App ──────────────────────────────────────────

# 编译 Go 后端到 desktop/resources/backend/（桌面应用 sidecar）
desktop-build-backend:
	mkdir -p desktop/resources/backend
	cd $(BACKEND) && go build -ldflags "$(LDFLAGS)" -o ../desktop/resources/backend/ebook-server$(if $(filter windows,$(GOOS)),.exe,) .

# 构建桌面应用前端
desktop-build-frontend:
	cd desktop && npm install && npm run build:renderer

# 打包桌面应用安装包
desktop-package: desktop-build-backend desktop-build-frontend
	cd desktop && npm run package

# 开发模式运行桌面应用（需先 desktop-build-backend）
desktop-dev: desktop-build-backend
	cd desktop && npm run dev
```

- [ ] **Step 3: 验证构建流程**

Run: `make desktop-build-backend`
Expected: `desktop/resources/backend/ebook-server.exe` 生成

Run: `cd desktop && npm run build:renderer`
Expected: `desktop/dist/renderer/` 目录生成，包含 index.html 和 assets/

- [ ] **Step 4: Commit**

```bash
git add desktop/electron-builder.yml Makefile
git commit -m "build(desktop): 添加 electron-builder 打包配置与 Makefile 构建目标"
```

---

## 完成标准

全部 Task 完成后，应满足：

1. `cd desktop && npm run dev:renderer` → Vite dev server 启动，浏览器打开 http://localhost:5173 看到完整管理界面
2. `make desktop-build-backend` → Go 二进制编译到 `desktop/resources/backend/`
3. `cd desktop && npm run build` → Main Process TypeScript 编译 + Vue 前端构建
4. `cd desktop && npx vitest run` → 所有单元测试通过
5. 配置页能读写 config.yaml / .env，服务页能启停 Go sidecar
