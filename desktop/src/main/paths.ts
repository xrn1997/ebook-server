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
