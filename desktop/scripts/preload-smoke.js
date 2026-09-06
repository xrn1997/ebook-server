// 用真实 dist/preload 验证沙箱兼容性：preload 加载失败（module not found 等）时退出码 1
const { app, BrowserWindow } = require('electron')
const path = require('node:path')

app.whenReady().then(async () => {
  const win = new BrowserWindow({
    show: false,
    webPreferences: {
      preload: path.join(__dirname, '..', 'dist', 'preload', 'index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  })
  let preloadError = null
  win.webContents.on('preload-error', (_e, _p, err) => { preloadError = String(err) })
  await win.loadURL('data:text/html,<h1>probe</h1>')
  const api = await win.webContents.executeJavaScript('typeof window.electronAPI')
  win.destroy()
  if (preloadError || api !== 'object') {
    console.error(`[preload-smoke] FAIL preloadError=${preloadError} electronAPI=${api}`)
    app.exit(1)
    return
  }
  console.log('[preload-smoke] OK: window.electronAPI 已暴露')
  app.exit(0)
})
