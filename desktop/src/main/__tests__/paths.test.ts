import { describe, it, expect } from 'vitest'
import { getSidecarPath, getConfigPath, getEnvPath, getIconPath } from '../paths'
import path from 'node:path'

// 用户数据目录本身的解析不在本模块（主进程 app.setPath 固定为 appData/ebook-server），
// 这里只验证「给定目录 → 文件路径」的拼装与资源目录布局。
describe('paths', () => {
  it('getConfigPath 直接位于 userData 下（不再多套同名子目录）', () => {
    const p = getConfigPath('/fake/userData')
    expect(p).toBe(path.join('/fake/userData', 'config.yaml'))
  })

  it('getEnvPath 直接位于 userData 下', () => {
    const p = getEnvPath('/fake/userData')
    expect(p).toBe(path.join('/fake/userData', '.env'))
  })

  // 这些断言必须跟 Makefile 的实际产物目录与 electron-builder 的 extraResources 映射一致，
  // 否则 desktop-dev 会 spawn 一个不存在的路径。
  it('getSidecarPath 在开发模式指向 make desktop-build-backend 的产物目录', () => {
    const p = getSidecarPath('/project/root', false, 'win32')
    expect(p).toBe(path.join('/project/root', 'desktop', 'resources', 'backend', 'ebook-server.exe'))
  })

  it('getSidecarPath 在打包模式从 resources/backend 查找二进制', () => {
    // 打包时调用方传 process.resourcesPath，extraResources 把 resources/backend 投到 <resources>/backend
    const p = getSidecarPath('/packed/resources', true, 'win32')
    expect(p).toBe(path.join('/packed/resources', 'backend', 'ebook-server.exe'))
  })

  it('getSidecarPath 在 macOS 不带 .exe 后缀', () => {
    const p = getSidecarPath('/project/root', false, 'darwin')
    expect(p).toBe(path.join('/project/root', 'desktop', 'resources', 'backend', 'ebook-server'))
  })

  it('getSidecarPath 在 linux 不带 .exe 后缀', () => {
    const p = getSidecarPath('/project/root', false, 'linux')
    expect(p).toBe(path.join('/project/root', 'desktop', 'resources', 'backend', 'ebook-server'))
  })

  it('getIconPath 与 sidecar 同布局：开发在 desktop/resources，打包在 <resources> 根', () => {
    expect(getIconPath('/project/root', false)).toBe(
      path.join('/project/root', 'desktop', 'resources', 'icon.png'),
    )
    expect(getIconPath('/packed/resources', true)).toBe(
      path.join('/packed/resources', 'icon.png'),
    )
  })
})
