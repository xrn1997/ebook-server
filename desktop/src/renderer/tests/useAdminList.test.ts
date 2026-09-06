import { describe, it, expect } from 'vitest'
import { useAdminList, PAGE_SIZE } from '../src/useAdminList'
import type { Envelope } from '../../shared/types'
import type { Page } from '../src/api'

const okResp: Envelope<Page<{ id: number }>> = {
  code: '00000',
  data: { list: [{ id: 1 }, { id: 2 }], total: 137 },
}

/** 造一个记录了每次请求页码的 load，各页都回同一份假数据 */
function fakeLoader() {
  const seen: number[] = []
  const load = (params: { page: number }): Promise<Envelope<Page<{ id: number }>>> => {
    seen.push(params.page)
    return Promise.resolve(okResp)
  }
  return { seen, load }
}

describe('useAdminList', () => {
  it('成功时填充 rows 与 total', async () => {
    const { rows, total, error, loading, reload } = useAdminList(async () => okResp)

    await reload()

    expect(rows.value).toHaveLength(2)
    expect(total.value).toBe(137)
    expect(error.value).toBe('')
    expect(loading.value).toBe(false)
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

  it('网络异常被转成一句可读提示，不抛出', async () => {
    const { error, reload } = useAdminList(async () => {
      throw new Error('fetch failed')
    })

    await expect(reload()).resolves.toBeUndefined()
    expect(error.value).toContain('无法连接后端服务')
    expect(error.value).toContain('fetch failed')
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
})
