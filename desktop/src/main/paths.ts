import path from 'node:path'

/**
 * 应用与数据目录约定。
 *
 * 用户数据目录固定为系统的 appData/<app name>（Windows %APPDATA%/ebook-server/，
 * macOS ~/Library/Application Support/ebook-server/，Linux ~/.config/ebook-server/），
 * 由主进程在 ready 前 `app.setPath` 固定——Electron 默认按包名取名，dev 与打包后的
 * 名字不同，不改的话两边会落到不同目录，配置互不相通。config.yaml、.env、ebook.db
 * 都放在这一层，不再多套子目录。
 */

/** 配置文件路径 */
export function getConfigPath(appUserData: string): string {
  return path.join(appUserData, 'config.yaml')
}

/** .env 文件路径 */
export function getEnvPath(appUserData: string): string {
  return path.join(appUserData, '.env')
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
    // 打包后 projectRoot 传入 process.resourcesPath，electron-builder 把 resources/backend 投到 <resources>/backend
    return path.join(projectRoot, 'backend', binaryName)
  }
  // 开发模式：make desktop-build-backend 产出到 desktop/resources/backend/，与打包布局一致
  return path.join(projectRoot, 'desktop', 'resources', 'backend', binaryName)
}

/**
 * 应用图标路径（托盘与打包共用同一份 PNG）。
 * 目录布局与 getSidecarPath 一致：打包后经 extraResources 落在 <resources>/，开发时在 desktop/resources/。
 */
export function getIconPath(projectRoot: string, isPackaged: boolean): string {
  return isPackaged
    ? path.join(projectRoot, 'icon.png')
    : path.join(projectRoot, 'desktop', 'resources', 'icon.png')
}
