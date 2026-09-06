// @vitest-environment happy-dom
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import Config from '../src/views/Config.vue'
import type { AppConfig, ElectronAPI, GetConfigResult, SaveConfigResult } from '../../shared/types'

function baseConfig(): AppConfig {
  return {
    server: { port: 9090, mode: 'release' },
    database: { path: 'ebook.db' },
    jwt: { secret: 'jwt-s', expire_min: 120 },
    smtp: { host: 'smtp.qq.com', port: 465, username: 'a@qq.com', password: 'smtp-s', from: 'a@qq.com', insecure: false },
    admin: { username: 'admin', password: 'adm-p', jwt_secret: 'adm-j', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
    upload: { dir: 'uploads' },
    api_docs: { enabled: false },
  }
}

const saveCalls: AppConfig[] = []

async function mountConfig(
  getConfigResult: GetConfigResult,
  saveResult: SaveConfigResult = { ok: true, needRestart: true },
) {
  saveCalls.length = 0
  window.electronAPI = {
    getConfig: async () => getConfigResult,
    saveConfig: async (config: unknown) => {
      saveCalls.push(config as AppConfig)
      return saveResult
    },
    restartService: vi.fn(),
  } as unknown as ElectronAPI

  const wrapper = mount(Config)
  await flushPromises()
  return wrapper
}

async function clickSave(wrapper: ReturnType<typeof mount>): Promise<void> {
  const button = wrapper.findAll('button').find((b) => b.text().includes('保存配置'))
  expect(button, '找不到保存按钮').toBeTruthy()
  await button!.trigger('click')
  await flushPromises()
}

describe('配置页', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })

  it('首屏敏感字段掩码显示：空串 + 留空保持现值提示', async () => {
    const masked = baseConfig()
    masked.smtp.password = ''
    masked.jwt.secret = ''
    masked.admin.jwt_secret = ''
    masked.admin.password = ''
    const wrapper = await mountConfig({ config: masked })

    expect((wrapper.find('input[placeholder="smtp.qq.com"]').element as HTMLInputElement).value)
      .toBe('smtp.qq.com')
    expect((wrapper.find('input[placeholder="留空保持现值"]').element as HTMLInputElement).value)
      .toBe('')
    expect(wrapper.text()).toContain('留空保存 = 保持现值')
  })

  it('把用户改过的值送到主进程，数字字段仍是 number', async () => {
    const wrapper = await mountConfig({ config: baseConfig() })

    await wrapper.find('input[placeholder="smtp.qq.com"]').setValue('smtp.exmail.qq.com')
    await clickSave(wrapper)

    expect(saveCalls).toHaveLength(1)
    expect(saveCalls[0].smtp.host).toBe('smtp.exmail.qq.com')
    expect(typeof saveCalls[0].smtp.port).toBe('number')
  })

  it('保存成功后提示需要重启，并给出立即重启入口', async () => {
    const wrapper = await mountConfig({ config: baseConfig() })
    await clickSave(wrapper)

    expect(wrapper.text()).toContain('配置已保存')
    expect(wrapper.text()).toContain('立即重启服务')
  })

  it('保存被拒时列出主进程返回的每一条错误，不假装成功', async () => {
    const wrapper = await mountConfig(
      { config: baseConfig() },
      { errors: ['jwt.secret 不能为空', 'admin.listen_addr 不允许使用通配地址（ADR-0010 网络隔离）'] },
    )
    await clickSave(wrapper)

    expect(wrapper.text()).toContain('jwt.secret 不能为空')
    expect(wrapper.text()).toContain('通配地址')
    expect(wrapper.text()).not.toContain('配置已保存')
    expect(saveCalls).toHaveLength(1)
  })

  it('config.yaml 读不出来时显示原因，而不是一片空白', async () => {
    const wrapper = await mountConfig({ error: 'config.yaml 结构不完整或无法解析，请检查各配置段与必填键' })

    expect(wrapper.text()).toContain('结构不完整')
    expect(wrapper.find('input[placeholder="smtp.qq.com"]').exists()).toBe(false)
  })

})
