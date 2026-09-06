import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { fetchStats, fetchUsers, forgetToken } from '../src/api'
import type { AdminTokenPayload, RuntimeInfo } from '../../shared/types'

/** 记录每次 fetch 的 URL 与请求头，用于断言端点与 Authorization 的来源 */
const calls: Array<{ url: string; headers: Record<string, string> }> = []

const runtimeFixture: RuntimeInfo = {
  status: 'running',
  uptimeMs: 1000,
  endpoints: { apiPort: 8080, apiBaseUrl: 'http://127.0.0.1:8080', adminBaseUrl: 'http://127.0.0.1:9191/admin/api', adminOrigin: 'http://127.0.0.1:9191', dbPath: '/work/ebook.db' },
  dbExists: true,
  dbSizeBytes: 2048,
  configStale: false,
}

let tokenRequests: AdminTokenPayload[] = []
let getAdminToken: ReturnType<typeof vi.fn>
let getRuntimeInfo: ReturnType<typeof vi.fn>

function stubElectron(): void {
  getAdminToken = vi.fn(async () => tokenRequests.shift() ?? { token: 'tok' })
  getRuntimeInfo = vi.fn(async () => runtimeFixture)
  ;(globalThis as unknown as { window: unknown }).window = {
    electronAPI: {
      getAdminToken,
      getRuntimeInfo,
    },
  }
}

function stubFetch(envelopes: Array<{ code?: string; error?: string; data?: unknown }>): void {
  let index = 0
  vi.stubGlobal('fetch', vi.fn(async (url: string, init: { headers: Record<string, string> }) => {
    calls.push({ url, headers: init.headers })
    const envelope = envelopes[Math.min(index, envelopes.length - 1)]
    index += 1
    return { json: async () => envelope }
  }))
}

describe('renderer 后台 API 层', () => {
  beforeEach(() => {
    calls.length = 0
    tokenRequests = []
    stubElectron()
    forgetToken()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    delete (globalThis as unknown as { window: unknown }).window
  })

  it('端点与令牌都取自主进程，不再写死 9091', async () => {
    stubFetch([{ code: '00000', data: { users: 1, comments: 2 } }])
    const resp = await fetchStats()

    expect(resp.code).toBe('00000')
    expect(calls[0].url).toBe('http://127.0.0.1:9191/admin/api/stats')
    expect(calls[0].headers.Authorization).toBe('Bearer tok')
  })

  it('拿不到令牌时不发请求，并把主进程给出的原因透出来', async () => {
    tokenRequests = [{ error: '管理员账号或密码未设置，请在配置页填写后重启服务' }]
    stubFetch([{ code: '00000', data: {} }])
    const resp = await fetchStats()

    expect(calls).toHaveLength(0)
    expect(resp.error).toContain('管理员账号或密码未设置')
  })

  it('令牌被拒（A0403）时强制重登并重试一次', async () => {
    tokenRequests = [{ token: 'stale' }, { token: 'fresh' }]
    stubFetch([{ code: 'A0403', error: '缺少管理端认证令牌' }, { code: '00000', data: { list: [], total: 0 } }])

    const resp = await fetchUsers()

    expect(resp.code).toBe('00000')
    expect(calls).toHaveLength(2)
    expect(calls[1].headers.Authorization).toBe('Bearer fresh')
    // 第二次索取必须带 force，否则主进程会把同一个坏 token 再给一次
    expect(getAdminToken).toHaveBeenNthCalledWith(2, true)
  })

  it('重登后仍失败则把失败如实返回，不再无限重试', async () => {
    tokenRequests = [{ token: 'stale' }, { token: 'fresh' }]
    stubFetch([{ code: 'A0403', error: '缺少管理端认证令牌' }, { code: 'A0403', error: '仍然不行' }])

    const resp = await fetchUsers()

    expect(calls).toHaveLength(2)
    expect(resp.code).toBe('A0403')
  })

  it('并发请求只换取一次令牌（后台登录限流 5 次/分钟）', async () => {
    stubFetch([{ code: '00000', data: { users: 0, comments: 0 } }])
    tokenRequests = [{ token: 'only-once' }]

    await Promise.all([fetchStats(), fetchStats(), fetchStats()])

    expect(getAdminToken).toHaveBeenCalledTimes(1)
  })

  it('主进程未就绪（无端点）时给出可读错误', async () => {
    getRuntimeInfo.mockResolvedValue(null)
    const resp = await fetchStats()
    expect(resp.error).toContain('无法取得后台端点')
  })
})
