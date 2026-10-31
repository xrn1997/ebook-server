import { describe, expect, it, vi } from 'vitest'
import { PAGE_SIZE, useAdminList } from './useAdminList'
import type { Envelope, Page } from './api'

const okResp: Envelope<Page<{ id: number }>> = {
  code: '00000',
  data: { list: [{ id: 1 }, { id: 2 }], total: 137 },
}

/** 造一个记录了每次请求页码与每页条数的 load，各页都回同一份假数据 */
function fakeLoader() {
  const seen: number[] = []
  const sizes: number[] = []
  const load = (params: { page: number; pageSize: number }): Promise<Envelope<Page<{ id: number }>>> => {
    seen.push(params.page)
    sizes.push(params.pageSize)
    return Promise.resolve(okResp)
  }
  return { seen, sizes, load }
}

/** 手动了结的响应：用来构造「先发慢回、后发先至」的乱序时序 */
function gate() {
  let resolve!: (env: Envelope<Page<{ id: number }>>) => void
  const promise = new Promise<Envelope<Page<{ id: number }>>>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

function pageEnv(list: Array<{ id: number }>, total: number): Envelope<Page<{ id: number }>> {
  return { code: '00000', data: { list, total } }
}

describe('useAdminList', () => {
  it('成功时填充 rows 与 total', async () => {
    const { rows, total, error, reload } = useAdminList(async () => okResp)

    await reload()

    expect(rows.value).toHaveLength(2)
    expect(total.value).toBe(137)
    expect(error.value).toBe('')
  })

  it('业务失败时展示后端给的文案（而不是笼统的加载失败）', async () => {
    const { rows, error, reload } = useAdminList(async () => ({ code: 'A0403', error: '缺少管理端认证令牌' }))

    await reload()

    expect(error.value).toBe('缺少管理端认证令牌')
    expect(rows.value).toEqual([])
  })

  it('信封成功但 data 为空时不报错', async () => {
    const { total, error, reload } = useAdminList(async () => ({ code: '00000' }))

    await reload()

    expect(error.value).toBe('')
    expect(total.value).toBe(0)
  })

  it('load 抛出意外异常时兜底为通用提示，不冒充网络故障', async () => {
    // 网络失败由 api 层翻译成 C0500 信封返回，走不到这里；能抛上来的只有编程错误，
    // 把它说成「无法连接后端服务」会把真 bug 伪装成运维问题。
    const log = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { error, reload } = useAdminList(async () => {
      throw new TypeError('cannot read properties of undefined')
    })

    await expect(reload()).resolves.toBeUndefined()

    expect(error.value).toBe('加载失败')
    expect(error.value).not.toContain('无法连接')
    expect(log).toHaveBeenCalled()
    log.mockRestore()
  })

  it('分页：页长固定 20，总页数按页长计算，翻页把页码传给 load', async () => {
    // 137 条 / 20 = 7 页
    const { seen, load } = fakeLoader()
    const list = useAdminList(load)

    await list.reload()
    expect(seen).toEqual([1])

    await list.nextPage()
    await list.gotoPage(7)
    expect(seen).toEqual([1, 2, 7])
    expect(list.page.value).toBe(7)
    expect(list.pageCount.value).toBe(7)
    expect(list.hasNext.value).toBe(false)
    expect(list.hasPrev.value).toBe(true)
  })

  it('翻页越界被钳制在有效范围内，首尾按钮不可用', async () => {
    // 只有 2 条 = 1 页
    const singlePage: Envelope<Page<{ id: number }>> = {
      code: '00000',
      data: { list: [{ id: 1 }], total: 2 },
    }
    const seen: number[] = []
    const list = useAdminList<{ id: number }>((params) => {
      seen.push(params.page)
      return Promise.resolve(singlePage)
    })

    await list.reload()
    // nextPage、gotoPage(99)、prevPage 都停在第 1 页
    await list.nextPage()
    await list.gotoPage(99)
    await list.prevPage()
    expect(seen).toEqual([1, 1, 1, 1])
    expect(list.page.value).toBe(1)
    expect(list.hasNext.value).toBe(false)
    expect(list.hasPrev.value).toBe(false)
  })

  it('每页条数按 20 显式传给后台（后台缺省是 10，不能依赖）', async () => {
    const { sizes, load } = fakeLoader()
    const list = useAdminList(load)

    await list.reload()

    // 断言的是真正发出的请求参数，不是常量本身：把 PAGE_SIZE 改成 10 时这里必须红。
    expect(sizes).toEqual([20])
    expect(PAGE_SIZE).toBe(20)
  })

  it('成功信封但页面形状不对时报错，而不是静默显示空表', async () => {
    // 后端字段改名（list → items）时，`?? []` 会把契约漂移渲染成「没有匹配的行」。
    const { rows, error, reload } = useAdminList(
      async () => ({ code: '00000', data: { items: [{ id: 1 }], total: 1 } }) as never,
    )

    await reload()

    expect(rows.value).toEqual([])
    expect(error.value).toBe('列表数据格式不正确')
  })

  it('请求在飞时 loading 为真，最新一次落地后归假', async () => {
    const entry = gate()
    const list = useAdminList<{ id: number }>(() => entry.promise)

    const pending = list.reload()
    expect(list.loading.value).toBe(true)

    entry.resolve(pageEnv([{ id: 1 }], 1))
    await pending
    expect(list.loading.value).toBe(false)
  })

  it('乱序响应：先发的慢响应不得覆盖后发的新结果', async () => {
    const gates = [gate(), gate(), gate()]
    const seen: number[] = []
    let n = 0
    const list = useAdminList<{ id: number }>((params) => {
      seen.push(params.page)
      const entry = gates[n]
      n += 1
      return entry.promise
    })

    // 先把总页数撑起来（137 条 = 7 页），否则翻页会被钳回第 1 页
    const boot = list.reload()
    gates[0].resolve(pageEnv([{ id: 0 }], 137))
    await boot

    const slow = list.gotoPage(2) // 先发出、后落地
    const fast = list.reload() // 后发出、先落地
    gates[2].resolve(pageEnv([{ id: 9 }], 1))
    await fast
    gates[1].resolve(pageEnv([{ id: 1 }, { id: 2 }], 137))
    await slow

    expect(seen).toEqual([1, 2, 1])
    expect(list.rows.value).toEqual([{ id: 9 }])
    expect(list.total.value).toBe(1)
    expect(list.page.value).toBe(1)
    // 被丢弃的慢响应不得把 loading 提前或最后翻回假
    expect(list.loading.value).toBe(false)
  })
})
