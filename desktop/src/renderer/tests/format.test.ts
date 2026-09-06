import { describe, it, expect } from 'vitest'
import { baseName, formatBytes, formatUptime } from '../src/format'

describe('formatBytes', () => {
  it('按 1024 进制给出人可读单位', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(12.3 * 1024 * 1024)).toBe('12.3 MB')
    expect(formatBytes(2 * 1024)).toBe('2.0 KB')
    expect(formatBytes(3 * 1024 ** 3)).toBe('3.0 GB')
  })

  it('异常输入显示占位符而不是 NaN', () => {
    expect(formatBytes(-1)).toBe('—')
    expect(formatBytes(Number.NaN)).toBe('—')
  })
})

describe('formatUptime', () => {
  it('超过一小时按「2h 34m」展示（设计文档给出的形态）', () => {
    expect(formatUptime((2 * 3600 + 34 * 60) * 1000)).toBe('2h 34m')
  })

  it('一小时以内显示分秒，一分钟内只显示秒', () => {
    expect(formatUptime(34 * 60_000 + 5_000)).toBe('34m 05s')
    expect(formatUptime(12_000)).toBe('12s')
    expect(formatUptime(0)).toBe('0s')
  })

  it('负值与非法值给占位符', () => {
    expect(formatUptime(-1)).toBe('—')
    expect(formatUptime(Number.POSITIVE_INFINITY)).toBe('—')
  })
})

describe('baseName', () => {
  it('从 Windows 与 POSIX 路径中取出文件名', () => {
    expect(baseName('C:\\Users\\me\\AppData\\Roaming\\ebook-server\\ebook.db')).toBe('ebook.db')
    expect(baseName('/home/me/.config/ebook-server/ebook.db')).toBe('ebook.db')
    expect(baseName('')).toBe('—')
  })
})
