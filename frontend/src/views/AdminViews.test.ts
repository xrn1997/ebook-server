// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Users from './Users.vue'
import Comments from './Comments.vue'
import Logs from './Logs.vue'
import { must, pick } from '../fixture'

/**
 * 三张管理视图的集成测试：只在 fetch 边界打桩，
 * 查询串拼装、信封判断、两步删除与分页钳制都跑真实代码。
 */

interface Call {
  url: string
  method: string
}

const calls: Call[] = []
let queue: unknown[] = []

/** 按顺序消费响应队列；只剩一个元素时该元素对所有后续调用生效。 */
function stubAdmin(...replies: unknown[]): void {
  queue = [...replies]
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, method: init?.method ?? 'GET' })
      const reply = queue.length > 1 ? queue.shift() : queue[0]
      return new Response(JSON.stringify(reply), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }),
  )
}

function listEnvelope(list: Array<Record<string, unknown>>, total = list.length): unknown {
  return { code: '00000', data: { list, total } }
}

/** 挂起的 fetch 替身：响应由测试手动了结，用来观察「请求在飞」这段时间的界面 */
function stubAdminPending() {
  let settle!: (reply: unknown) => void
  const promise = new Promise<Response>((resolve) => {
    settle = (reply: unknown) => {
      resolve(
        new Response(JSON.stringify(reply), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }
  })
  vi.stubGlobal('fetch', vi.fn(async () => promise))
  return { settle }
}

function userRow(uid: number, email: string): Record<string, unknown> {
  return { uid, email, username: `u${uid}`, nickname: '', avatar: '', created_at: 't' }
}

beforeEach(() => {
  calls.length = 0
  queue = []
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Users.vue', () => {
  it('搜索把关键字与分页参数带给 /users', async () => {
    stubAdmin(listEnvelope([userRow(1, 'a@x.com')]), listEnvelope([userRow(1, 'a@x.com')]))
    const wrapper = mount(Users)
    await flushPromises()

    await wrapper.find('input').setValue('alice')
    const search = must(wrapper.findAll('button').find((b) => b.text() === '搜索'), '「搜索」按钮')
    await search.trigger('click')
    await flushPromises()

    expect(calls.at(-1)?.url).toContain('/admin/api/users?page=1&page_size=20&keyword=alice')
    expect(wrapper.text()).toContain('a@x.com')
  })

  it('点详情拉取 /users/:uid 并展示邮箱与评论数', async () => {
    stubAdmin(
      listEnvelope([userRow(7, 'u@x.com')]),
      {
        code: '00000',
        data: {
          user: { uid: 7, email: 'u@x.com', username: 'u', nickname: '昵称', avatar: '', created_at: 't' },
          comment_count: 3,
        },
      },
    )
    const wrapper = mount(Users)
    await flushPromises()

    const detail = must(wrapper.findAll('button').find((b) => b.text() === '详情'), '「详情」按钮')
    await detail.trigger('click')
    await flushPromises()

    expect(calls.at(-1)?.url).toBe('/admin/api/users/7')
    expect(wrapper.text()).toContain('评论数')
    expect(wrapper.text()).toContain('昵称')
  })

  it('首屏请求在飞时显示加载中，落地后消失且不再重复', async () => {
    const pending = stubAdminPending()
    const wrapper = mount(Users)
    await flushPromises()

    expect(wrapper.text()).toContain('加载中')
    // 加载期间不能同时渲染空表：那会让「还没有数据」与「正在取数据」看起来一样。
    expect(wrapper.text()).not.toContain('没有匹配的账号')

    pending.settle(listEnvelope([userRow(1, 'a@x.com')]))
    await flushPromises()

    expect(wrapper.text()).not.toContain('加载中')
    expect(wrapper.text()).toContain('a@x.com')
  })

  it('单页列表没有下一页，并如实显示总数', async () => {
    stubAdmin(listEnvelope([userRow(1, 'a@x.com')]))
    const wrapper = mount(Users)
    await flushPromises()

    const next = must(wrapper.findAll('button').find((b) => b.text() === '下一页'), '「下一页」按钮')
    expect((next.element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.text()).toContain('共 1 个账号')
  })

  it('重新搜索清掉上一轮的详情卡片', async () => {
    stubAdmin(
      listEnvelope([userRow(7, 'u@x.com')]),
      { code: '00000', data: { user: userRow(7, 'u@x.com'), comment_count: 3 } },
      listEnvelope([userRow(8, 'v@x.com')]),
    )
    const wrapper = mount(Users)
    await flushPromises()

    await must(wrapper.findAll('button').find((b) => b.text() === '详情'), '「详情」按钮').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('评论数')

    await must(wrapper.findAll('button').find((b) => b.text() === '搜索'), '「搜索」按钮').trigger('click')
    await flushPromises()

    // 详情属于旧筛选条件的结果，不能挂在新列表旁边
    expect(wrapper.text()).not.toContain('评论数')
    expect(wrapper.text()).toContain('v@x.com')
  })

  it('连续点开两个详情：先发后回的慢响应不得覆盖新详情', async () => {
    // 列表立即落地；两个详情请求各自手动了结，让第一个（uid 7）后回——
    // 竞态丢弃已下沉进 useAdminDetail，这里钉住视图接线后行为不变。
    function json(reply: unknown): Response {
      return new Response(JSON.stringify(reply), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }
    const detailGates: Array<(reply: unknown) => void> = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        calls.push({ url, method: 'GET' })
        // 列表是 /users?query，详情是 /users/:uid
        if (!url.includes('/users/')) return json(listEnvelope([userRow(7, 'a@x.com'), userRow(8, 'b@x.com')]))
        return new Promise<Response>((resolve) => {
          detailGates.push((reply) => {
            resolve(json(reply))
          })
        })
      }),
    )
    const wrapper = mount(Users)
    await flushPromises()

    const details = () => wrapper.findAll('button').filter((b) => b.text() === '详情')
    await pick(details(), 0, 'details()').trigger('click') // 先点 uid 7：慢回
    await pick(details(), 1, 'details()').trigger('click') // 再点 uid 8：先回

    pick(detailGates, 1, 'detailGates')({
      code: '00000',
      data: { user: { ...userRow(8, 'b@x.com'), nickname: '快八' }, comment_count: 8 },
    })
    await flushPromises()
    pick(detailGates, 0, 'detailGates')({
      code: '00000',
      data: { user: { ...userRow(7, 'a@x.com'), nickname: '慢七' }, comment_count: 7 },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('快八')
    expect(wrapper.text()).not.toContain('慢七')
  })
})

describe('Comments.vue', () => {
  function commentRow(id: number): Record<string, unknown> {
    return {
      id,
      user_id: 1,
      user: { username: 'u' },
      content: `内容${id}`,
      chapter_name: '',
      book_name: '书A',
      created_at: 't',
    }
  }

  it('删除走两步确认：第一次只点亮按钮，第二次才交 DELETE 并刷新列表', async () => {
    stubAdmin(listEnvelope([commentRow(11)]), { code: '00000' }, listEnvelope([]))
    const wrapper = mount(Comments)
    await flushPromises()

    const delButton = () => must(wrapper.findAll('button').find((b) => b.text().includes('删除')), '「删除」按钮')
    expect(delButton().text()).toBe('删除')
    await delButton().trigger('click')
    expect(delButton().text()).toBe('确认删除')
    expect(calls).toHaveLength(1) // 只有列表那次

    await delButton().trigger('click')
    await flushPromises()

    expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/comments/11'))).toBe(true)
    // 删除成功后回到第 1 页刷新列表
    expect(calls.at(-1)?.method).toBe('GET')
    expect(calls.at(-1)?.url).toContain('page=1')
    expect(wrapper.text()).toContain('没有匹配的评论')
  })

  it('翻页作废未完成的删除确认', async () => {
    stubAdmin(listEnvelope([commentRow(11)], 40), listEnvelope([commentRow(12)], 40))
    const wrapper = mount(Comments)
    await flushPromises()

    await must(wrapper.findAll('button').find((b) => b.text().includes('删除')), '「删除」按钮').trigger('click')
    expect(wrapper.text()).toContain('确认删除')

    const next = must(wrapper.findAll('button').find((b) => b.text() === '下一页'), '「下一页」按钮')
    expect((next.element as HTMLButtonElement).disabled).toBe(false)
    await next.trigger('click')
    await flushPromises()

    // 换了一页行之后，「确认删除」不能还挂在某个同 id 的行上
    expect(wrapper.text()).not.toContain('确认删除')
    expect(wrapper.text()).toContain('内容12')
  })

  it('删除失败展示后端文案且保留现有行', async () => {
    // 后台治理删不做归属校验；它唯一会回的评论域错误是「不存在」（A0304）。
    stubAdmin(listEnvelope([commentRow(11)]), { code: 'A0304', error: '评论不存在' })
    const wrapper = mount(Comments)
    await flushPromises()

    const del = must(wrapper.findAll('button').find((b) => b.text().includes('删除')), '「删除」按钮')
    await del.trigger('click')
    await del.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('评论不存在')
    expect(wrapper.text()).toContain('内容11')
  })

  it('加载期间不渲染空表', async () => {
    const pending = stubAdminPending()
    const wrapper = mount(Comments)
    await flushPromises()

    expect(wrapper.text()).toContain('加载中')
    expect(wrapper.text()).not.toContain('没有匹配的评论')

    pending.settle(listEnvelope([]))
    await flushPromises()

    expect(wrapper.text()).toContain('没有匹配的评论')
  })

  it('搜索把内容关键字与书名带给 /comments', async () => {
    stubAdmin(listEnvelope([]), listEnvelope([]))
    const wrapper = mount(Comments)
    await flushPromises()

    const inputs = wrapper.findAll('input')
    await pick(inputs, 0, 'inputs').setValue('精彩')
    await pick(inputs, 1, 'inputs').setValue('书A')
    const search = must(wrapper.findAll('button').find((b) => b.text() === '搜索'), '「搜索」按钮')
    await search.trigger('click')
    await flushPromises()

    expect(calls.at(-1)?.url).toContain('keyword=')
    expect(calls.at(-1)?.url).toContain('book_name=')
  })
})

describe('Logs.vue', () => {
  function logRow(): Record<string, unknown> {
    return {
      id: 1,
      user_id: 0,
      username: '',
      method: 'POST',
      path: '/api/comments',
      ip: '',
      user_agent: 'smoke-agent',
      response_code: 200,
      error_code: 'A0303',
      error_message: '',
      created_at: 't',
    }
  }

  it('勾选「只看失败」带 failed=true，取消后不再带', async () => {
    stubAdmin(listEnvelope([logRow()]), listEnvelope([logRow()]), listEnvelope([logRow()]))
    const wrapper = mount(Logs)
    await flushPromises()

    const checkbox = wrapper.find('input[type="checkbox"]')
    expect(checkbox.exists()).toBe(true)
    await checkbox.setValue(true)
    await flushPromises()
    expect(calls.at(-1)?.url).toContain('failed=true')

    await checkbox.setValue(false)
    await flushPromises()
    expect(calls.at(-1)?.url).not.toContain('failed=')
  })

  it('方法下拉与路径关键字参与筛选', async () => {
    stubAdmin(listEnvelope([]), listEnvelope([]))
    const wrapper = mount(Logs)
    await flushPromises()

    await wrapper.find('select').setValue('POST')
    await wrapper.find('input:not([type="checkbox"])').setValue('/comments')
    const filter = must(wrapper.findAll('button').find((b) => b.text() === '筛选'), '「筛选」按钮')
    await filter.trigger('click')
    await flushPromises()

    expect(calls.at(-1)?.url).toContain('method=POST')
    expect(calls.at(-1)?.url).toContain('path=')
  })

  it('加载期间不渲染空表', async () => {
    const pending = stubAdminPending()
    const wrapper = mount(Logs)
    await flushPromises()

    expect(wrapper.text()).toContain('加载中')
    expect(wrapper.text()).not.toContain('没有匹配的记录')

    pending.settle(listEnvelope([]))
    await flushPromises()

    expect(wrapper.text()).toContain('没有匹配的记录')
  })

  it('详情里的用户优先显示用户名，匿名请求才落到 UID 或 -', async () => {
    stubAdmin(listEnvelope([{ ...logRow(), user_id: 7, username: 'alice' }]))
    const wrapper = mount(Logs)
    await flushPromises()

    await wrapper.find('tbody tr').trigger('click')

    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).not.toContain('7')
  })

  it('点击行展开详情，再点收起', async () => {
    stubAdmin(listEnvelope([logRow()]))
    const wrapper = mount(Logs)
    await flushPromises()

    expect(wrapper.text()).not.toContain('smoke-agent')
    await wrapper.find('tbody tr').trigger('click')
    expect(wrapper.text()).toContain('A0303')
    expect(wrapper.text()).toContain('smoke-agent')

    await wrapper.find('tbody tr').trigger('click')
    expect(wrapper.text()).not.toContain('smoke-agent')
  })
})
