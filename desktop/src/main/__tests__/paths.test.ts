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
