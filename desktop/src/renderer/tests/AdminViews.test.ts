// @vitest-environment happy-dom
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Users from '../src/views/Users.vue'
import Comments from '../src/views/Comments.vue'
import Logs from '../src/views/Logs.vue'
import type { Page } from '../src/api'
import type { AdminTokenPayload, Envelope, RuntimeInfo } from '../../shared/types'

/**
 * 三个管理视图（用户/评论/日志）的搜索、分页、删除、详情、筛选。
 * 走真实 api 层：stub 的只有 window.electronAPI 与全局 fetch，信封判断与
 * 查询串拼装都是真代码（「能跑真东西就不打桩」）。
 */

const runtime: RuntimeInfo = {
  status: 'running',
  uptimeMs: 1000,
  endpoints: { apiPort: 8080, apiBaseUrl: 'http://127.0.0.1:8080', adminBaseUrl: 'http://127.0.0.1:9191/admin/api', adminOrigin: 'http://127.0.0.1:9191', dbPath: '/w/db' },
  dbExists: true,
  dbSizeBytes: 1,
  configStale: false,
}

const calls: Array<{ url: string; init: RequestInit }> = []

/**
 * 按顺序消费响应队列；只有一个元素时该元素对所有请求生效。
 * 每次请求都会先记进 calls，再回队列里的下一个信封。
 */
function stubFetch(envelopes: Envelope<unknown> | Array<Envelope<unknown>>): void {
  const queue = Array.isArray(envelopes) ? [...envelopes] : [envelopes]
  vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit) => {
    calls.push({ url, init })
    const envelope = queue.length > 1 ? queue.shift()! : queue[0]
    return { json: async () => envelope }
  }))
}

function stubElectron(token: AdminTokenPayload = { token: 'jwt' }): void {
  // happy-dom 环境下只挂属性，不能整体替换 window（会丢掉 Event 等构造器）
  ;(window as unknown as { electronAPI: unknown }).electronAPI = {
    getRuntimeInfo: async () => runtime,
    getAdminToken: async () => token,
  }
}

function listEnvelope(list: Array<Record<string, unknown>>): Envelope<Page<Record<string, unknown>>> {
  return { code: '00000', data: { list, total: list.length } }
}

beforeEach(() => {
  calls.length = 0
  stubElectron()
})

afterEach(() => {
  vi.unstubAllGlobals()
  delete (window as unknown as { electronAPI?: unknown }).electronAPI
})

describe('Users.vue', () => {
  it('搜索把关键字与 snake_case 分页参数带给 /users', async () => {
    stubFetch(listEnvelope([
      { uid: 1, email: 'a@x.com', username: 'a', nickname: '', avatar: '', created_at: 't' },
    ]))
    const wrapper = mount(Users)
    await flushPromises()

    await wrapper.find('input').setValue('alice')
    await wrapper.findAll('button').find((b) => b.text() === '搜索')!.trigger('click')
    await flushPromises()

    expect(calls.at(-1)?.url).toContain('/users?page=1&page_size=20&keyword=alice')
    expect(wrapper.text()).toContain('a@x.com')
  })

  it('点详情拉取 /users/:uid 并展示邮箱与评论数', async () => {
    stubFetch([
      listEnvelope([{ uid: 7, email: 'u@x.com', username: 'u', nickname: '', avatar: '', created_at: 't' }]),
      { code: '00000', data: { user: { uid: 7, email: 'u@x.com', username: 'u', nickname: '昵称', avatar: '', created_at: 't' }, comment_count: 3 } },
    ])
    const wrapper = mount(Users)
    await flushPromises()

    await wrapper.findAll('a').find((a) => a.text() === '详情')!.trigger('click')
    await flushPromises()

    expect(calls.at(-1)?.url).toContain('/users/7')
    expect(wrapper.text()).toContain('评论数')
    expect(wrapper.text()).toContain('昵称')
  })

  it('单页列表没有下一页，并如实显示总数', async () => {
    stubFetch(listEnvelope([{ uid: 1, email: 'a@x.com', username: 'a', nickname: '', avatar: '', created_at: 't' }]))
    const wrapper = mount(Users)
    await flushPromises()

    const next = wrapper.findAll('button').find((b) => b.text() === '下一页')!
    expect((next.element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.text()).toContain('共 1 个账号')
  })
})

describe('Comments.vue', () => {
  function commentRow(id: number): Record<string, unknown> {
    return { id, user_id: 1, user: { username: 'u' }, content: `内容${id}`, chapter_name: '', book_name: '书A', created_at: 't' }
  }

  it('删除走两步确认：第一次只点亮按钮，第二次才发 DELETE 并刷新列表', async () => {
    stubFetch([
      listEnvelope([commentRow(11)]),
      { code: '00000' },
      listEnvelope([]),
    ])
    const wrapper = mount(Comments)
    await flushPromises()

    const delButton = () => wrapper.findAll('button').find((b) => b.text().includes('删除'))!
    expect(delButton().text()).toBe('删除')
    await delButton().trigger('click')
    expect(delButton().text()).toBe('确认删除')
    expect(calls).toHaveLength(1) // 只有列表那次

    await delButton().trigger('click')
    await flushPromises()

    expect(calls.some((c) => c.init.method === 'DELETE' && c.url.endsWith('/comments/11'))).toBe(true)
    // 删除成功后回到第 1 页刷新列表
    expect(calls.at(-1)?.init.method).toBe('GET')
    expect(calls.at(-1)?.url).toContain('page=1')
    expect(wrapper.text()).toContain('没有匹配的评论')
  })

  it('搜索把内容关键字与书名带给 /comments', async () => {
    stubFetch(listEnvelope([]))
    const wrapper = mount(Comments)
    await flushPromises()

    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('精彩')
    await inputs[1].setValue('书A')
    await wrapper.findAll('button').find((b) => b.text() === '搜索')!.trigger('click')
    await flushPromises()

    expect(calls.at(-1)?.url).toContain('keyword=')
    expect(calls.at(-1)?.url).toContain('book_name=')
  })
})

describe('Logs.vue', () => {
  it('勾选「只看失败」后筛选参数带 failed=true', async () => {
    stubFetch(listEnvelope([
      { id: 1, user_id: 0, username: '', method: 'POST', path: '/api/comments', ip: '', user_agent: '', response_code: 200, error_code: 'A0303', error_message: '', created_at: 't' },
    ]))
    const wrapper = mount(Logs)
    await flushPromises()

    await wrapper.find('input[type="checkbox"]').setValue(true)
    await flushPromises()

    expect(calls.at(-1)?.url).toContain('failed=true')
    expect(wrapper.text()).toContain('A0303')
  })

  it('方法下拉与路径关键字参与筛选', async () => {
    stubFetch(listEnvelope([]))
    const wrapper = mount(Logs)
    await flushPromises()

    await wrapper.find('select').setValue('POST')
    await wrapper.find('input:not([type="checkbox"])').setValue('/comments')
    await wrapper.findAll('button').find((b) => b.text() === '筛选')!.trigger('click')
    await flushPromises()

    expect(calls.at(-1)?.url).toContain('method=POST')
    expect(calls.at(-1)?.url).toContain('path=')
  })
})
