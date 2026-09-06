import { app, BrowserWindow, Tray, Menu, nativeImage } from 'electron'
import path from 'node:path'
import fs from 'node:fs'
import { SidecarManager } from './sidecar'
import { addLog, registerIpcHandlers, unregisterIpcHandlers } from './ipc'
import { AdminTokenStore } from './admin-auth'
import { deriveEndpoints, readManagedConfig, writeFullConfig, type AppConfig } from './config'
import { getSidecarPath, getConfigPath, getEnvPath, getIconPath } from './paths'
import { Events } from '../shared/channels'
import type { ServiceErrorReason, ServiceStatus } from '../shared/types'

// 用户数据目录固定为 appData/ebook-server（Windows %APPDATA%/ebook-server/）。
// Electron 默认按包名取名：dev 用 package.json 的 name（ebook-server-desktop）、
// 打包用 productName（ebook-server），不改的话开发与安装版落到不同目录，
// 配置与数据库互不相通；必须在 ready 之前设置才会生效。
app.setName('ebook-server')
app.setPath('userData', path.join(app.getPath('appData'), 'ebook-server'))

// E2E 把 userData 指到临时目录，避免污染真实用户数据
const userDataOverride = process.env.EBOOK_SERVER_USER_DATA
if (userDataOverride) {
  app.setPath('userData', userDataOverride)
}

// 主进程崩溃的最后一道记录：config/.env/db 都在 userData，crash.log 与它们同处一地。
// 渲染层崩溃走 Chromium 自己的上报，这里只兜主进程。
process.on('uncaughtException', (error) => {
  console.error('[FATAL] Uncaught exception:', error)
  try {
    fs.appendFileSync(
      path.join(app.getPath('userData'), 'crash.log'),
      `[${new Date().toISOString()}] ${error.stack || error}\n`,
    )
  } catch { /* 连日志都写不进去时只能放弃 */ }
})

let mainWindow: BrowserWindow | null = null
let tray: Tray | null = null
let sidecar: SidecarManager

const isDev = !app.isPackaged

/** sidecar 工作目录 = 用户数据目录：config.yaml / .env / ebook.db 都在这里 */
function userDataDir(): string {
  return app.getPath('userData')
}

/**
 * 当前**运行中的进程**所用配置快照。
 * 后端只在启动时读一次配置，所以正在监听的端口、以及它接受的 ADMIN_PASSWORD，
 * 都以这份快照为准；磁盘上的新配置要等重启才生效。健康检查与自动登录都读它。
 */
let runtimeConfig: AppConfig | null = null
/** 磁盘配置已保存但尚未重启生效 */
let configStale = false

function createWindow(): void {
  mainWindow = new BrowserWindow({
    width: 1100,
    height: 750,
    minWidth: 900,
    minHeight: 600,
    title: 'ebook-server',
    show: false,
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  })

  if (isDev && !process.env.EBOOK_SERVER_RENDERER_FILE) {
    mainWindow.loadURL('http://localhost:5173')
  } else {
    mainWindow.loadFile(
      process.env.EBOOK_SERVER_RENDERER_FILE || path.join(__dirname, '..', 'renderer', 'index.html'),
    )
  }

  // 渲染层被注入脚本时，window.open / 任意导航是把内容送出应用的通道：
  // 一律拒绝，只允许加载本应用自己的页面
  mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
  mainWindow.webContents.on('will-navigate', (event, url) => {
    const allowed = isDev
      ? url.startsWith('http://localhost:5173')
      : url.startsWith('file://')
    if (!allowed) event.preventDefault()
  })

  mainWindow.once('ready-to-show', () => {
    mainWindow?.show()
  })

  mainWindow.on('closed', () => {
    mainWindow = null
  })
}

function createTray(): void {
  // 托盘与安装包共用同一份图标（extraResources 投到 <resources>/icon.png）。
  // 图标缺失时退回 1x1 透明 PNG：Windows 上 Tray 吃到空图会直接崩。
  const projectRoot = isDev ? path.join(app.getAppPath(), '..') : process.resourcesPath
  const iconPath = getIconPath(projectRoot, app.isPackaged)
  const icon = fs.existsSync(iconPath)
    ? nativeImage.createFromPath(iconPath)
    : nativeImage.createFromBuffer(TRANSPARENT_PNG)
  tray = new Tray(icon)
  tray.setToolTip('ebook-server')

  const contextMenu = Menu.buildFromTemplate([
    { label: '显示主窗口', click: () => mainWindow?.show() },
    { type: 'separator' },
    {
      label: '重启服务',
      click: () => { void applyServiceConfig('restart') },
    },
    { type: 'separator' },
    { label: '退出', click: () => app.quit() },
  ])
  tray.setContextMenu(contextMenu)
  tray.on('click', () => mainWindow?.show())
}

/** 1x1 透明 PNG，仅作图标文件缺失时的占位（见 createTray） */
const TRANSPARENT_PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVQI12NgAAIABQABNjN9GQAAAAlwSFlzAAAWJQAAFiUBSVIk8AAAAA0lEQVQI12P4z8BQDwAEgAF/QualIQAAAABJRU5ErkJggg==',
  'base64',
)

function ensureUserData(): void {
  fs.mkdirSync(userDataDir(), { recursive: true })

  const configPath = getConfigPath(userDataDir())
  if (!fs.existsSync(configPath)) {
    const templatePath = isDev
      ? path.join(app.getAppPath(), 'templates', 'config.yaml')
      : path.join(process.resourcesPath, 'templates', 'config.yaml')
    if (fs.existsSync(templatePath)) {
      fs.copyFileSync(templatePath, configPath)
    }
  }

  const envPath = getEnvPath(userDataDir())
  if (!fs.existsSync(envPath)) {
    fs.writeFileSync(envPath, '', 'utf-8')
  }
}

function sendToRenderer(channel: string, data: unknown): void {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send(channel, data)
  }
}

/** 把失败原因转成界面可读提示（设计文档要求区分端口被占用等具体原因） */
function reportError(reason: ServiceErrorReason, message: string): void {
  sendToRenderer(Events.SERVICE_ERROR, { reason, message })
}

/** 管理后台 token 提供者：凭据与端点都取自运行中的配置快照 */
const adminAuth = new AdminTokenStore(() => {
  if (!runtimeConfig) return null
  return {
    adminBaseUrl: deriveEndpoints(runtimeConfig, userDataDir()).adminBaseUrl,
    username: runtimeConfig.admin.username,
    password: runtimeConfig.admin.password,
  }
})

/**
 * 服务就绪后自动登录后台，预热 token 缓存。
 * 为什么这么做：用户/评论/日志三个视图走的是 /admin/api/*，全部在 ADR-0010 的
 * 独立后台鉴权之后；而管理员凭据本来就由本应用托管，再做一次手动登录没有意义。
 * 令牌本身由 renderer 按需 invoke 取用，失败原因才需要主动告知界面。
 */
async function warmAdminToken(): Promise<void> {
  const result = await adminAuth.obtain(true)
  if (!result.token && result.error) {
    reportError('admin-login-failed', result.error)
  }
}

/**
 * 用磁盘上的最新配置驱动 sidecar 启动或重启。
 * 启动与重启此前是两份只差一句话的函数，配置读取、快照更新、token 失效必须
 * 完全一致，收敛成一处。
 */
async function applyServiceConfig(mode: 'start' | 'restart'): Promise<void> {
  const action = mode === 'start' ? '启动' : '重启'
  const config = readManagedConfig(getConfigPath(userDataDir()), getEnvPath(userDataDir()))
  if (!config) {
    reportError('config-invalid', `config.yaml 缺失或结构不完整，未${action}后端。请打开配置页修正后重试。`)
    return
  }
  runtimeConfig = config
  configStale = false
  // 运行中的进程马上要没了/已经没了，它签发的 token 不再有意义
  adminAuth.invalidate()
  if (mode === 'start') {
    sidecar.start()
  } else {
    await sidecar.restart()
  }
}

function bootSidecar(): void {
  const projectRoot = isDev ? path.join(app.getAppPath(), '..') : process.resourcesPath
  const binaryPath = getSidecarPath(projectRoot, app.isPackaged, process.platform)

  sidecar = new SidecarManager({
    binaryPath,
    workDir: userDataDir(),
    port: () => runtimeConfig?.server.port ?? 0,
    onStatusChange: (status: ServiceStatus) => {
      sendToRenderer(Events.STATUS_CHANGED, { status })
      if (status === 'running') void warmAdminToken()
      // stopped 与 error 都意味着「这份 token 背后的登录已经无效」（ADR-0012 §3：
      // token 生命周期跟随子进程），error 时也要作废，否则崩溃重启后 renderer
      // 还拿着旧 token 撞 A0403。
      if (status === 'stopped' || status === 'error') adminAuth.invalidate()
    },
    onLog: (line: string) => {
      addLog(line)
      sendToRenderer(Events.LOG_LINE, { line })
    },
    onError: (reason, message) => reportError(reason, message),
  })

  registerIpcHandlers({
    sidecar,
    startService: () => { void applyServiceConfig('start') },
    restartService: () => applyServiceConfig('restart'),
    configPath: getConfigPath(userDataDir()),
    envPath: getEnvPath(userDataDir()),
    workDir: userDataDir(),
    auth: adminAuth,
    getRuntimeConfig: () => runtimeConfig,
    isConfigStale: () => configStale,
    persistConfig: (config) => {
      writeFullConfig(getConfigPath(userDataDir()), getEnvPath(userDataDir()), config)
      configStale = true
    },
  })

  void applyServiceConfig('start')
}

// 单实例锁：第二个实例直接退出，避免两份进程抢同一个 sidecar 端口与数据库
const gotLock = app.requestSingleInstanceLock()
if (!gotLock) {
  app.quit()
}

app.on('second-instance', () => {
  if (mainWindow) {
    if (mainWindow.isMinimized()) mainWindow.restore()
    mainWindow.focus()
  }
})

app.whenReady().then(() => {
  if (!gotLock) return
  try {
    ensureUserData()
    createWindow()
    createTray()
    bootSidecar()
  } catch (error) {
    console.error('[main] Startup error:', error)
  }
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

// 退出时先等 sidecar 落定再真正退出：before-quit 的处理是同步的，此刻直接返回会让
// Electron 立刻收尾、把正在优雅退出中的子进程强杀；preventDefault + 轮询状态、
// 到期再 app.quit() 才能让「写 stdin shutdown → 5 秒宽限」的停止流程走完。
let quitting = false
app.on('before-quit', (event) => {
  if (quitting) return
  quitting = true
  event.preventDefault()
  unregisterIpcHandlers()
  sidecar?.stop()
  const deadline = Date.now() + 6_000
  const tick = (): void => {
    const stillStopping = sidecar !== undefined && sidecar.getStatus() === 'stopping'
    if (stillStopping && Date.now() < deadline) {
      setTimeout(tick, 100)
      return
    }
    app.quit()
  }
  tick()
})
