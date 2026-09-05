import { app, BrowserWindow, Tray, Menu, nativeImage } from 'electron'
import path from 'node:path'
import fs from 'node:fs'
import { SidecarManager, type SidecarStatus } from './sidecar'
import { registerIpcHandlers, unregisterIpcHandlers, addLog } from './ipc'
import { Channels } from './ipc'
import { getSidecarPath, getUserDataDir, getConfigPath, getEnvPath } from './paths'

process.on('uncaughtException', (error) => {
  console.error('[FATAL] Uncaught exception:', error)
  const logPath = path.join(app.getPath('userData'), 'crash.log')
  try {
    fs.appendFileSync(logPath, `[${new Date().toISOString()}] ${error.stack || error}\n`)
  } catch { /* ignore */ }
})

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
    show: false,
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

  mainWindow.once('ready-to-show', () => {
    mainWindow?.show()
  })

  mainWindow.on('closed', () => {
    mainWindow = null
  })
}

function createTray(): void {
  // Windows 上 nativeImage.createEmpty() 会导致 Tray 崩溃，用 1x1 透明 PNG 替代
  const transparentPng = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVQI12NgAAIABQABNjN9GQAAAAlwSFlzAAAWJQAAFiUBSVIk8AAAAA0lEQVQI12P4z8BQDwAEgAF/QualIQAAAABJRU5ErkJggg==',
    'base64',
  )
  const icon = nativeImage.createFromBuffer(transparentPng)
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

function sendToRenderer(channel: string, data: unknown): void {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send(channel, data)
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
    onStatusChange: (status: SidecarStatus) => {
      sendToRenderer(Channels.STATUS_CHANGED, { status })
    },
    onLog: (line: string) => {
      addLog(line)
      sendToRenderer(Channels.LOG_LINE, { line })
    },
  })

  registerIpcHandlers(sidecar, app.getPath('userData'))
  sidecar.start()
}

app.whenReady().then(() => {
  try {
    ensureUserData()
    createWindow()
    createTray()
    startSidecar()
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

app.on('before-quit', () => {
  sidecar?.stop()
  unregisterIpcHandlers()
})
