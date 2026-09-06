/** 展示用的格式化函数（概览页与测试共用，故独立成模块而不是写在 .vue 里） */

const KB = 1024
const MB = KB * 1024
const GB = MB * 1024

/**
 * 把字节数格式化为人可读单位，保留一位小数。
 * 设计文档要求概览页显示「数据库：ebook.db (12.3 MB)」这种形态。
 */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '—'
  if (bytes < KB) return `${bytes} B`
  if (bytes < MB) return `${(bytes / KB).toFixed(1)} KB`
  if (bytes < GB) return `${(bytes / MB).toFixed(1)} MB`
  return `${(bytes / GB).toFixed(1)} GB`
}

/**
 * 把毫秒运行时长格式化为人可读文本。
 * 超过 1 小时用「2h 34m」，1 小时内用「34m 05s」，1 分钟内用「12s」。
 */
export function formatUptime(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '—'
  const totalSeconds = Math.floor(ms / 1000)
  const seconds = totalSeconds % 60
  const minutes = Math.floor(totalSeconds / 60) % 60
  const hours = Math.floor(totalSeconds / 3600)
  if (hours > 0) return `${hours}h ${String(minutes).padStart(2, '0')}m`
  if (minutes > 0) return `${minutes}m ${String(seconds).padStart(2, '0')}s`
  return `${seconds}s`
}

/** 从路径中取出文件名，用于「数据库：ebook.db (12.3 MB)」这种展示 */
export function baseName(filePath: string): string {
  if (!filePath) return '—'
  const parts = filePath.split(/[/\\]/)
  return parts[parts.length - 1] || filePath
}
