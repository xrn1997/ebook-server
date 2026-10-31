import { describe, expect, it, vi } from 'vitest'

/**
 * api 模块的令牌契约，跑在默认 node 环境（文件首行不标注 happy-dom）。
 *
 * 这一份测试钉的是「模块可导入」而不是「请求发得对」：后台视图、单元测试、
 * 任何非浏览器消费者都会 `import './api'`，只要模块初始化期去读一次浏览器全局，
 * 整个模块就在无 DOM 的环境里直接炸掉——本次 vitest 门禁红就是这么来的。
 */

/** 极简存储替身：读写都落在一个 Map 上，可被测试从外部直接改动。 */
function fakeStorage() {
  const map = new Map<string, string>()
  return {
    map,
    getItem: (k: string) => map.get(k) ?? null,
    setItem: (k: string, v: string) => {
      map.set(k, v)
    },
    removeItem: (k: string) => {
      map.delete(k)
    },
  }
}

describe('api 模块的令牌存取契约', () => {
  it('浏览器全局缺失时也能导入，读作空令牌、写不抛', async () => {
    vi.resetModules()
    vi.stubGlobal('localStorage', undefined)

    const api = await import('./api')

    expect(api.token.value).toBe('')
    expect(() => api.token.set('abc')).not.toThrow()
    expect(api.token.value).toBe('')

    vi.unstubAllGlobals()
  })

  it('令牌是活读：导入之后外部写入存储的值立刻可见', async () => {
    vi.resetModules()
    const store = fakeStorage()
    vi.stubGlobal('localStorage', store)

    const api = await import('./api')
    expect(api.token.value).toBe('')

    // 导入之后再改存储：快照式实现会一直停留在导入时刻的空值
    store.map.set('ebook_admin_token', 'external')
    expect(api.token.value).toBe('external')

    vi.unstubAllGlobals()
  })

  it('set 写入存储、set("") 清除，令牌跟着存储走', async () => {
    vi.resetModules()
    const store = fakeStorage()
    vi.stubGlobal('localStorage', store)

    const api = await import('./api')
    api.token.set('issued')

    expect(store.map.get('ebook_admin_token')).toBe('issued')
    expect(api.token.value).toBe('issued')

    api.token.set('')
    expect(store.map.has('ebook_admin_token')).toBe(false)
    expect(api.token.value).toBe('')

    vi.unstubAllGlobals()
  })
})
