import { describe, it, expect, beforeAll, afterAll } from 'vitest'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { SidecarManager } from '../sidecar'
import { AdminTokenStore, loginAsAdmin } from '../admin-auth'
import { deriveEndpoints, writeFullConfig, type AppConfig } from '../config'
import { getConfigPath, getEnvPath, getSidecarPath } from '../paths'

/**
 * 主进程模块 ↔ 真实 Go 后端的集成测试（设计文档 测试策略：「集成测试：Electron + Go 通信、
 * 服务生命周期」）。不打桩：真起子进程、真读健康检查、真发 HTTP 登录。
 *
 * 需要 sidecar 二进制存在：`make desktop-build-backend`。缺失时按仓库惯例跳过而不是伪造通过。
 */

// desktop/src/main/__tests__ → 上四级才是仓库根
const repoRoot = path.resolve(__dirname, '..', '..', '..', '..')
const binaryPath = getSidecarPath(repoRoot, false, process.platform)
const hasBinary = fs.existsSync(binaryPath)

// 高位随机端口，避开本机常驻的 9090/9091
const apiPort = 20000 + Math.floor(Math.random() * 20000)
const adminPort = apiPort + 1

let workDir: string
let config: AppConfig
let sidecar: SidecarManager | null = null

function baseConfig(): AppConfig {
  return {
    server: { port: apiPort, mode: 'release' },
    database: { path: 'ebook.db' },
    jwt: { secret: 'it-jwt-secret', expire_min: 120 },
    smtp: { host: 'smtp.invalid.example', port: 465, username: 'no-reply@example.com', password: 'it-smtp-pass', from: 'no-reply@example.com', insecure: true },
    admin: { username: 'it-admin', password: 'it-admin-pass', jwt_secret: 'it-admin-jwt', expire_min: 60, listen_addr: '127.0.0.1', listen_port: adminPort },
    upload: { dir: 'uploads' },
    api_docs: { enabled: false },
  }
}

async function waitFor(predicate: () => boolean, ms: number): Promise<boolean> {
  const deadline = Date.now() + ms
  while (Date.now() < deadline) {
    if (predicate()) return true
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  return false
}

beforeAll(async () => {
  if (!hasBinary) return
  // 用户数据目录本身（appData/ebook-server）由主进程 setPath 决定，集成测试直接用临时目录当工作目录
  workDir = fs.mkdtempSync(path.join(os.tmpdir(), 'desktop-it-'))
  fs.mkdirSync(workDir, { recursive: true })
  config = baseConfig()
  writeFullConfig(getConfigPath(workDir), getEnvPath(workDir), config)

  sidecar = new SidecarManager({
    binaryPath,
    workDir,
    port: () => config.server.port,
    onStatusChange: () => {},
    onLog: () => {},
    onError: () => {},
  })
  sidecar.start()
  // 真二进制首次启动要建表（AutoMigrate），给的余量比单元测试里的 10 秒超时更宽
  expect(await waitFor(() => sidecar?.getStatus() === 'running', 25_000)).toBe(true)
}, 60_000)

afterAll(async () => {
  if (sidecar) {
    sidecar.stop()
    await waitFor(() => sidecar?.getStatus() === 'stopped', 15_000)
  }
  if (workDir) fs.rmSync(workDir, { recursive: true, force: true })
}, 60_000)

const maybe = hasBinary ? describe : describe.skip

maybe('主进程 ↔ 真实 Go 后端集成', () => {
  it('健康检查拨的是配置里的端口，不是写死的 9090', async () => {
    const res = await fetch(`http://127.0.0.1:${apiPort}/health`)
    const body = (await res.json()) as { code?: string }
    expect(body.code).toBe('00000')
  })

  it('后台端点在 ADR-0010 的鉴权之后：无令牌被拒', async () => {
    const body = (await (await fetch(`http://127.0.0.1:${adminPort}/admin/api/users`)).json()) as { code?: string }
    expect(body.code).toBe('A0403')
  })

  it('用受管配置自动登录即可拉通用户/评论/日志三个视图所需的接口', async () => {
    const endpoints = deriveEndpoints(config, workDir)
    const store = new AdminTokenStore(() => ({
      adminBaseUrl: endpoints.adminBaseUrl,
      username: config.admin.username,
      password: config.admin.password,
    }))
    const { token, error } = await store.obtain()
    expect(error).toBeUndefined()

    for (const api of ['/users', '/comments', '/logs', '/stats']) {
      const body = (await fetch(endpoints.adminBaseUrl + api, {
        headers: { Authorization: 'Bearer ' + token },
      }).then((r) => r.json())) as { code?: string; error?: string }
      expect(body.code, `${api} 失败：${body.error}`).toBe('00000')
    }
  })

  it('管理员密码不匹配时自动登录失败并给出后台文案', async () => {
    const endpoints = deriveEndpoints(config, workDir)
    const result = await loginAsAdmin(endpoints.adminBaseUrl, config.admin.username, '不对的密码')
    expect(result.token).toBeUndefined()
    expect(result.error).toContain('密码错误')
  })

  it('SQLite 建在 sidecar 工作目录，供概览页显示大小', () => {
    const endpoints = deriveEndpoints(config, workDir)
    expect(fs.existsSync(endpoints.dbPath)).toBe(true)
    expect(fs.statSync(endpoints.dbPath).size).toBeGreaterThan(0)
  })

  it('端口已被占用时第二个 sidecar 报「端口被占用」，且不会误显示为运行中', async () => {
    // 第二个实例用独立工作目录：否则它会先撞上 SQLite 文件锁，验证不到 bind 失败这条路径
    const secondWorkDir = fs.mkdtempSync(path.join(os.tmpdir(), 'desktop-it-clash-'))
    fs.mkdirSync(secondWorkDir, { recursive: true })
    writeFullConfig(getConfigPath(secondWorkDir), getEnvPath(secondWorkDir), {
      ...config,
      database: { path: 'clash.db' },
      admin: { ...config.admin, listen_port: adminPort + 1 },
    })

    const errors: Array<{ reason: string; message: string }> = []
    const statuses: string[] = []
    const clashing = new SidecarManager({
      binaryPath,
      workDir: secondWorkDir,
      port: () => apiPort, // 与第一个实例抢同一个公开端口
      onStatusChange: (s) => statuses.push(s),
      onLog: () => {},
      onError: (reason, message) => errors.push({ reason, message }),
    })
    clashing.start()

    const detected = await waitFor(() => errors.some((e) => e.reason === 'port-conflict'), 25_000)
    clashing.stop()
    fs.rmSync(secondWorkDir, { recursive: true, force: true })

    expect(detected, `实际收到的错误：${JSON.stringify(errors)}`).toBe(true)
    expect(errors.find((e) => e.reason === 'port-conflict')?.message).toContain('被占用')
    // 占用者会正确回答 /health，若把它当自己的就绪就会出现下面的假 running
    expect(statuses).not.toContain('running')
  }, 40_000)

  it('修改配置并重启后，健康检查拨的是新端口（配置页的关键用户流程）', async () => {
    const newPort = apiPort + 7
    config = { ...config, server: { ...config.server, port: newPort } }
    writeFullConfig(getConfigPath(workDir), getEnvPath(workDir), config)

    await sidecar!.restart()
    expect(await waitFor(() => sidecar?.getStatus() === 'running', 25_000)).toBe(true)

    // 端口来源是受管配置：旧端口不再有服务，新端口正常应答
    await expect(fetch(`http://127.0.0.1:${apiPort}/health`)).rejects.toThrow()
    const body = (await (await fetch(`http://127.0.0.1:${newPort}/health`)).json()) as { code?: string }
    expect(body.code).toBe('00000')
  }, 40_000)

  it('停止走优雅退出：远快于 5 秒强杀宽限期，且端口释放、运行时长归零', async () => {
    expect(sidecar?.getStatus()).toBe('running')
    expect(sidecar?.getUptimeMs()).toBeGreaterThan(0)

    const startedAt = Date.now()
    sidecar?.stop()
    // 真二进制收到了 stdin 的 shutdown 指令：自己 Shutdown 并退出，
    // 而不是「Windows 下 taskkill 不带 /F 无效 → 干等 5 秒 → 被 /F 击杀」
    expect(await waitFor(() => sidecar?.getStatus() === 'stopped', 20_000)).toBe(true)
    expect(Date.now() - startedAt).toBeLessThan(5_000)
    expect(sidecar?.getUptimeMs()).toBe(0)

    await expect(fetch(`http://127.0.0.1:${config.server.port}/health`)).rejects.toThrow()
  }, 30_000)
})
