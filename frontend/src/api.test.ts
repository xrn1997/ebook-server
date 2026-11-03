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
  unwrap,
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
    // 替身签名收窄到 `string`：本仓所有调用都传 `BASE + path`，写成 fetch 的全签名反而让
    // String(url) 有把 Request 变成 '[object Object]' 的空间。
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
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
    // body 只可能是 JSON.stringify 出来的串；不是串就直接炸这条用例，
    // 而不是 String() 成 '[object Object]' 再让 JSON.parse 报一句难读的错。
    const body = calls[0]?.init?.body
    if (typeof body !== 'string') throw new Error(`期望请求体是字符串，实得 ${typeof body}`)
    expect(JSON.parse(body)).toEqual({ username: 'admin', password: 'secret' })
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

describe('unwrap', () => {
  it('成功信封解出 ok 与 data', async () => {
    const result = await unwrap(
      Promise.resolve({ code: '00000', data: { users: 1, comments: 2 } }),
      '加载失败',
    )

    expect(result.ok).toBe(true)
    expect(result.data).toEqual({ users: 1, comments: 2 })
    expect(result.message).toBe('')
  })

  it('失败信封带 error 时透传后端文案，而不是退回兜底', async () => {
    const result = await unwrap(
      Promise.resolve({ code: 'A0304', error: '评论不存在' }),
      '删除评论失败',
    )

    expect(result.ok).toBe(false)
    expect(result.data).toBeUndefined()
    expect(result.message).toBe('评论不存在')
  })

  it('失败信封缺 error 字段时用调用方传入的 fallback', async () => {
    const result = await unwrap(Promise.resolve({ code: 'A0120' }), '加载用户详情失败')

    expect(result.ok).toBe(false)
    expect(result.message).toBe('加载用户详情失败')
  })

  it('省略兜底文案时使用默认文案', async () => {
    const result = await unwrap(Promise.resolve({ code: 'A0120' }))

    expect(result.ok).toBe(false)
    expect(result.message).toBe('操作失败')
  })
})
