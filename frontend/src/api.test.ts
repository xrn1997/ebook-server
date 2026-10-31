// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  deleteComment,
  fetchComments,
  fetchLogs,
  fetchStats,
  fetchUserDetail,
  fetchUsers,
  login,
  token,
} from './api'

interface Call {
  url: string
  init: RequestInit | undefined
}

const calls: Call[] = []

/** 只在 fetch 边界打桩：查询串拼装、信封判断、鉴权跳转都跑真实代码。 */
function stubFetch(reply: unknown): void {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      calls.push({ url: String(url), init })
      return new Response(JSON.stringify(reply), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }),
  )
}

beforeEach(() => {
  calls.length = 0
  token.set('')
  window.location.hash = ''
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('后台 API 调用层', () => {
  it('查询串用 snake_case、省掉空值与 false，并带 Bearer', async () => {
    token.set('t')
    stubFetch({ code: '00000', data: { list: [], total: 0 } })

    await fetchUsers({ page: 2, pageSize: 20, keyword: '' })
    await fetchUserDetail(7)
    await fetchComments({ page: 1, keyword: '精彩', bookName: '书A' })
    await fetchLogs({ page: 1, method: 'POST', failed: false })
    await fetchLogs({ page: 1, failed: true })
    await deleteComment(11)

    expect(calls.map((c) => c.url)).toEqual([
      '/admin/api/users?page=2&page_size=20',
      '/admin/api/users/7',
      '/admin/api/comments?page=1&keyword=%E7%B2%BE%E5%BD%A9&book_name=%E4%B9%A6A',
      '/admin/api/logs?page=1&method=POST',
      '/admin/api/logs?page=1&failed=true',
      '/admin/api/comments/11',
    ])
    for (const call of calls) {
      expect(new Headers(call.init?.headers).get('Authorization')).toBe('Bearer t')
    }
    expect(calls.at(-1)?.init?.method).toBe('DELETE')
  })

  it('未登录时不带 Authorization', async () => {
    stubFetch({ code: '00000', data: { users: 1, comments: 2 } })

    await fetchStats()

    expect(new Headers(calls[0]?.init?.headers).get('Authorization')).toBeNull()
  })

  it('登录以 JSON 提交账号密码并返回 token', async () => {
    stubFetch({ code: '00000', data: { token: 'jwt' } })

    const resp = await login('admin', 'secret')

    expect(calls[0]?.url).toBe('/admin/api/login')
    expect(calls[0]?.init?.method).toBe('POST')
    expect(JSON.parse(String(calls[0]?.init?.body))).toEqual({ username: 'admin', password: 'secret' })
    expect(resp.data?.token).toBe('jwt')
  })

  it('非登录请求鉴权失败时清 token 并回登录页', async () => {
    token.set('expired')
    stubFetch({ code: 'A0403', error: '令牌过期' })

    const resp = await fetchUsers()

    expect(resp.code).toBe('A0403')
    expect(token.value).toBe('')
    expect(window.location.hash).toBe('#/login')
  })

  it('登录失败不清 token、不跳转', async () => {
    token.set('keep')
    stubFetch({ code: 'A0403', error: '账号或密码错误' })

    await login('admin', 'wrong')

    expect(token.value).toBe('keep')
    expect(window.location.hash).toBe('')
  })

  it('网络异常被转成可读的 C0500 信封，不抛给调用方', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('boom')
      }),
    )

    const resp = await fetchStats()

    expect(resp.code).toBe('C0500')
    expect(resp.error).toContain('无法连接后端服务')
    expect(resp.error).toContain('boom')
  })

  it('网关的 HTML 错误页被转成 C0500，不把 JSON 解析错误原文展示给管理员', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('<html>Bad Gateway</html>', { status: 502 })),
    )

    const resp = await fetchStats()

    expect(resp.code).toBe('C0500')
    expect(resp.error).toContain('HTTP 502')
    expect(resp.error).not.toContain('Unexpected token')
  })

  it('HTTP 200 但不是信封时回兜底文案', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>index</html>', { status: 200 })))

    const resp = await fetchStats()

    expect(resp.code).toBe('C0500')
    expect(resp.error).toBe('后端返回了无法解析的响应')
  })

  it('HTTP 200 且是合法 JSON 但不是对象信封时也不抛给视图', async () => {
    // 代理/静态服务可能回 200 + `null`：`as Envelope` 会在视图里变成 TypeError，
    // 所以边界必须自己判形状，而不是只判能否 JSON.parse。
    stubFetch(null)

    const resp = await fetchStats()

    expect(resp.code).toBe('C0500')
    expect(resp.error).toBe('后端返回了无法识别的响应')
  })
})
