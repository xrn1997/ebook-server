// @vitest-environment happy-dom
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Overview from '../src/views/Overview.vue'
import { serviceStatus } from '../src/stores/service'
import { forgetToken } from '../src/api'
import type { ElectronAPI, RuntimeInfo } from '../../shared/types'

const runtime: RuntimeInfo = {
  status: 'running',
  uptimeMs: (2 * 3600 + 34 * 60) * 1000,
  endpoints: {
    apiPort: 8080,
    apiBaseUrl: 'http://127.0.0.1:8080',
    adminBaseUrl: 'http://192.168.1.10:9191/admin/api',
    adminOrigin: 'http://192.168.1.10:9191',
    dbPath: 'C:\\Users\\me\\ebook.db',
  },
  dbExists: true,
  dbSizeBytes: 12.3 * 1024 * 1024,
  configStale: true,
}

async function mountOverview(overrides: Partial<RuntimeInfo> = {}) {
  window.electronAPI = {
    getRuntimeInfo: async () => ({ ...runtime, ...overrides }),
    getAdminToken: async () => ({ token: 'jwt' }),
    restartService: vi.fn(),
    stopService: vi.fn(),
  } as unknown as ElectronAPI
  vi.stubGlobal('fetch', vi.fn(async () => ({
    json: async () => ({ code: '00000', data: { users: 3, comments: 5 } }),
  })))
  const wrapper = mount(Overview)
  await flushPromises()
  return wrapper
}

describe('概览页', () => {
  beforeEach(() => {
    serviceStatus.value = 'running'
    forgetToken()
    vi.unstubAllGlobals()
  })

  it('展示设计文档要求的运行监控：端口、数据库大小、运行时间', async () => {
    const text = (await mountOverview()).text()

    expect(text).toContain('http://127.0.0.1:8080')
    expect(text).toContain('http://192.168.1.10:9191')
    expect(text).toContain('ebook.db (12.3 MB)')
    expect(text).toContain('2h 34m')
  })

  it('展示后台返回的统计数字', async () => {
    const wrapper = await mountOverview()

    const labels = wrapper.findAll('.stat-card .label').map((el) => el.text())
    const numbers = wrapper.findAll('.stat-card .num').map((el) => el.text())

    expect(labels).toEqual(expect.arrayContaining(['服务状态：运行中', '注册用户', '评论总数']))
    expect(numbers).toEqual(['●', '3', '5'])
  })

  it('配置已保存但未重启时提示需要重启', async () => {
    const text = (await mountOverview()).text()
    expect(text).toContain('需重启服务')
  })

  it('数据库文件尚未创建时说明未创建而不是显示 0 B', async () => {
    const text = (await mountOverview({ dbExists: false, dbSizeBytes: 0 })).text()
    expect(text).toContain('尚未创建')
  })

  it('自动登录失败时把主进程给的原因说出来（而不是静默显示破折号）', async () => {
    window.electronAPI = {
      getRuntimeInfo: async () => runtime,
      getAdminToken: async () => ({ error: '管理员账号或密码未设置，请在配置页填写后重启服务' }),
      restartService: vi.fn(),
      stopService: vi.fn(),
    } as unknown as ElectronAPI
    // 拿不到令牌就不该发出请求，这里放一个必定让测试失败的桩
    const fetchMock = vi.fn(async () => ({ json: async () => ({ code: 'SHOULD_NOT_BE_CALLED' }) }))
    vi.stubGlobal('fetch', fetchMock)

    const wrapper = mount(Overview)
    await flushPromises()

    expect(wrapper.text()).toContain('管理员账号或密码未设置')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
