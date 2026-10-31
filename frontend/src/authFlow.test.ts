// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import App from './App.vue'
import Dashboard from './views/Dashboard.vue'
import Login from './views/Login.vue'
import router from './router'
import { SUCCESS_CODE, token } from './api'

/**
 * 后台鉴权链路的组件级用例。
 *
 * AGENTS.md 要求「鉴权失败立即清除 token 并回登录页」，而这条链路横跨三处：
 * api 层清 token + 改 hash、路由守卫拦无令牌的导航、视图与存储的实际读写。
 * 只测其中任何一环都会让整条链断掉而门禁仍是绿的，所以这里从公共边界（真实 router、
 * 真实存储替身、fetch 边界）走一遍。
 */

interface Call {
  url: string
}

const calls: Call[] = []

/** 按顺序消费响应；队列耗尽后重复最后一个，避免断言脆弱的调用次数。 */
function stubReplies(...replies: unknown[]): void {
  const queue = [...replies]
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string | URL | Request) => {
      calls.push({ url: String(url) })
      const reply = queue.length > 1 ? queue.shift() : queue[0]
      return new Response(JSON.stringify(reply), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }),
  )
}

const okStats = { code: SUCCESS_CODE, data: { users: 12, comments: 34 } }
const authExpired = { code: 'A0403', error: '管理端令牌已失效' }

beforeEach(async () => {
  calls.length = 0
  token.set('')
  vi.unstubAllGlobals()
  window.location.hash = '#/login'
  await router.push('/login')
})

describe('路由守卫', () => {
  it('无令牌时任何受保护路由都被送回登录页', async () => {
    stubReplies(okStats)

    await router.push('/users')

    expect(router.currentRoute.value.name).toBe('login')
    // 守卫在挂载前就拦下，不该有后台请求发出
    expect(calls).toHaveLength(0)
  })

  it('带令牌时放行到目标路由', async () => {
    stubReplies(okStats)
    token.set('issued')

    await router.push('/users')

    expect(router.currentRoute.value.path).toBe('/users')
  })

  it('令牌中途失效：清掉存储里的令牌，之后的导航仍然回登录页', async () => {
    token.set('expired-but-present')
    stubReplies(authExpired)

    // 必须真的挂载出 <router-view>：只有视图发出那次请求，才能观察到 A0403 的后果
    mount(App, { global: { plugins: [router] } })
    await router.push('/logs')
    await flushPromises()

    expect(token.value).toBe('')
    expect(window.location.hash).toBe('#/login')

    // 完整链路：守卫读的是同一个存储，清完就不会再放进任何受保护路由
    await router.push('/comments')
    expect(router.currentRoute.value.name).toBe('login')
  })
})

describe('Login.vue', () => {
  it('登录成功把令牌写进存储并跳概览', async () => {
    stubReplies({ code: SUCCESS_CODE, data: { token: 'jwt-issued' } })
    const wrapper = mount(Login, { global: { plugins: [router] } })

    await wrapper.findAll('input')[0].setValue('admin')
    await wrapper.findAll('input')[1].setValue('secret')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(token.value).toBe('jwt-issued')
    expect(router.currentRoute.value.path).toBe('/')
  })

  it('登录失败展示后端文案、不写令牌、不跳转', async () => {
    // 后台登录失败统一回 A0403（adminAuthFailureCode），A0241 是「尝试超限」；
    // 用端点真实会回的码，失败路径才是真的走了一遍。
    stubReplies({ code: 'A0403', error: '账号或密码错误' })
    const wrapper = mount(Login, { global: { plugins: [router] } })

    await wrapper.findAll('input')[0].setValue('admin')
    await wrapper.findAll('input')[1].setValue('wrong')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.text()).toContain('账号或密码错误')
    expect(token.value).toBe('')
    expect(router.currentRoute.value.path).toBe('/login')
  })
})

describe('App.vue', () => {
  it('退出登录清空存储并回到登录页', async () => {
    stubReplies(okStats)
    token.set('issued')
    await router.push('/')
    const wrapper = mount(App, { global: { plugins: [router] } })
    await flushPromises()

    await wrapper.find('button.logout').trigger('click')

    expect(token.value).toBe('')
    expect(window.location.hash).toBe('#/login')
  })
})

describe('Dashboard.vue', () => {
  it('展示用户数与评论总数', async () => {
    stubReplies(okStats)

    const wrapper = mount(Dashboard)
    await flushPromises()

    expect(calls[0]?.url).toBe('/admin/api/stats')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('34')
    expect(wrapper.text()).not.toContain('加载中')
  })

  it('概览拉取失败时显示后端文案而不是停在加载中', async () => {
    stubReplies({ code: 'C0500', error: '无法连接后端服务' })

    const wrapper = mount(Dashboard)
    await flushPromises()

    expect(wrapper.text()).toContain('无法连接后端服务')
    expect(wrapper.text()).not.toContain('加载中')
  })
})
