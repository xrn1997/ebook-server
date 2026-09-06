import { test, expect, _electron, type ElectronApplication, type Page } from '@playwright/test'
import { mkdtempSync, existsSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const projectRoot = join(__dirname, '..', '..')
// 本仓库桌面测试只在 Windows 跑：Go 二进制名带 .exe（与 Makefile 的 GOOS 判断一致），非 Windows 上整段 skip
const sidecarBinary = join(projectRoot, 'desktop', 'resources', 'backend', 'ebook-server.exe')
const mainEntry = join(projectRoot, 'desktop', 'dist', 'main', 'index.js')
const rendererFile = join(projectRoot, 'desktop', 'dist', 'renderer', 'index.html')

test.skip(!existsSync(sidecarBinary) || !existsSync(mainEntry), '需要先 make desktop-build-backend 与构建 renderer 产物')

let app: ElectronApplication
let window: Page
let userData: string
let closed = false

/** 幂等关闭并返回耗时（ms），供退出时序断言与 afterEach 复用 */
async function closeApp(): Promise<number> {
  if (closed) return 0
  closed = true
  const start = Date.now()
  await app.close()
  return Date.now() - start
}

test.beforeEach(async () => {
  userData = mkdtempSync(join(tmpdir(), 'ebook-e2e-'))
  closed = false
  // 模板 config.yaml 的敏感字段为空串，过不了校验；E2E 需要预填有效值
  writeFileSync(join(userData, 'config.yaml'), [
    'server:',
    '  port: 19090',
    '  mode: release',
    'database:',
    '  path: ebook.db',
    'api_docs:',
    '  enabled: false',
    'jwt:',
    '  secret: e2e-jwt-secret',
    '  expire_min: 120',
    'admin:',
    '  username: admin',
    '  password: e2e-admin-pw',
    '  jwt_secret: e2e-adm-jwt',
    '  expire_min: 60',
    '  listen_addr: 127.0.0.1',
    '  listen_port: 19091',
    'smtp:',
    '  host: ""',
    '  port: 465',
    '  username: ""',
    '  password: ""',
    '  from: ""',
    '  insecure: false',
    'upload:',
    '  dir: uploads',
  ].join('\n') + '\n', 'utf-8')
  writeFileSync(join(userData, '.env'), '', 'utf-8')
  app = await _electron.launch({
    args: [join(__dirname, '..')],
    env: {
      ...process.env,
      EBOOK_SERVER_USER_DATA: userData,
      EBOOK_SERVER_RENDERER_FILE: rendererFile,
    },
  })
  window = await app.firstWindow()
})

test.afterEach(async () => {
  await closeApp()
  rmSync(userData, { recursive: true, force: true })
})

test('启动后概览可见、sidecar 进入运行中', async () => {
  await expect(window.locator('h2')).toContainText('概览')
  await expect(window.getByText('服务状态：运行中')).toBeVisible({ timeout: 30_000 })
  await expect(window.getByText(/ebook\.db/)).toBeVisible()
})

test('渲染页带 CSP meta（Task 6 的真实行为）', async () => {
  const hasCsp = await window.evaluate(
    () => !!document.querySelector('meta[http-equiv="Content-Security-Policy"]'),
  )
  expect(hasCsp).toBe(true)
})

test('退出时等待 sidecar 落定而不是立即收尾（Task 7 的真实行为）', async () => {
  await expect(window.getByText('服务状态：运行中')).toBeVisible({ timeout: 30_000 })
  const elapsed = await closeApp()
  // Task 7 落地前是同步 stop 后立即收尾（耗时≈0）；落地后至少经一轮轮询，且不得烧满 6 秒 deadline
  expect(elapsed).toBeLessThan(6_000)
})
