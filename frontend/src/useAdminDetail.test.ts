import { describe, expect, it, vi } from 'vitest'
import { useAdminDetail } from './useAdminDetail'
import type { Envelope } from './api'
import { pick } from './fixture'

interface Detail {
  uid: number
}

/** 手动了结的响应：用来构造「先发慢回、后发先至」的乱序时序（同 useAdminList.test 的 gate） */
function gate() {
  let resolve!: (env: Envelope<Detail>) => void
  const promise = new Promise<Envelope<Detail>>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

describe('useAdminDetail', () => {
  it('sets data on success', async () => {
    // fetcher 收到的就是 load() 传进来的那一个 id：参数化正是为了钉住这条传递，
    // 而不是让视图靠闭包里的可变变量在事后被读到。
    const detail = useAdminDetail<Detail, number>(
      async (uid) => ({ code: '00000', data: { uid } }),
      '加载用户详情失败',
    )

    await detail.load(7)

    expect(detail.data.value).toEqual({ uid: 7 })
    expect(detail.error.value).toBe('')
    expect(detail.loading.value).toBe(false)
  })

  it('surfaces fallback message on failure', async () => {
    // 后端带文案时透传后端文案（对外行为不变，同 useAdminList 的失败语义）；
    // 缺 error 字段时才落到调用方传入的 fallback。
    const withText = useAdminDetail<Detail, number>(
      async () => ({ code: 'A0403', error: 'x' }),
      '加载用户详情失败',
    )
    await withText.load(7)
    expect(withText.error.value).toBe('x')

    const bare = useAdminDetail<Detail, number>(async () => ({ code: 'A0120' }), '加载用户详情失败')
    await bare.load(7)
    expect(bare.error.value).toBe('加载用户详情失败')
    expect(bare.data.value).toBeUndefined()
  })

  it('鉴权失败时清空上一次详情，只留原因', async () => {
    // token 清理与跳登录住 request 层（A0403 → AUTH_FAILURE_CODES），本层只保证
    // 详情区不会带着 A0403 的文案继续显示上一个账号——换账号后旧详情已经不适用。
    let code = '00000'
    const detail = useAdminDetail<Detail, number>(
      async (uid) => (code === '00000' ? { code, data: { uid } } : { code }),
      '加载用户详情失败',
    )

    await detail.load(7)
    expect(detail.data.value).toEqual({ uid: 7 })

    code = 'A0403'
    await detail.load(8)
    expect(detail.data.value).toBeUndefined()
    expect(detail.error.value).toBe('加载用户详情失败')
    expect(detail.loading.value).toBe(false)
  })

  it('成功却没带 data：报数据格式不正确，不留一块空白详情', async () => {
    // data 缺席是契约漂移（后端改字段名、或无载荷成功时整键被 omitempty 掉）。只判 ok 会让
    // error 与 data 同时为空、loading 落为 false，Users.vue 的 v-if/v-else-if 三链皆假——
    // 详情区整块消失，把后端改动渲染成「这条记录没有内容」。
    const detail = useAdminDetail<Detail, number>(async () => ({ code: '00000' }), '加载用户详情失败')

    await detail.load(7)

    expect(detail.data.value).toBeUndefined()
    expect(detail.error.value).toBe('详情数据格式不正确')
    expect(detail.loading.value).toBe(false)
  })

  it('fetcher 抛编程错误：只留通用文案，原文进控制台', async () => {
    // api 层已经把网络失败翻译成信封，能抛到这里的只剩编程错误；说成运维问题会把真 bug
    // 藏起来。这条分支此前只靠读代码确认，没有可失败的断言钉着。
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
    const detail = useAdminDetail<Detail, number>(
      async () => {
        throw new TypeError('programming error')
      },
      '加载用户详情失败',
    )

    await detail.load(7)

    expect(detail.data.value).toBeUndefined()
    expect(detail.error.value).toBe('加载用户详情失败')
    expect(detail.loading.value).toBe(false)
    expect(logged).toHaveBeenCalledWith('后台详情加载抛出意外异常', expect.any(TypeError))
    logged.mockRestore()
  })

  it('discards stale response', async () => {
    const gates = [gate(), gate()]
    let n = 0
    const detail = useAdminDetail<Detail, number>(() => pick(gates, n++).promise, '加载用户详情失败')

    const first = detail.load(1) // 先发、后落地
    const second = detail.load(2) // 后发、先落地

    pick(gates, 1).resolve({ code: '00000', data: { uid: 2 } })
    await second
    pick(gates, 0).resolve({ code: '00000', data: { uid: 1 } })
    await first

    // 慢回的旧响应整段丢弃：data 只认第二次的结果
    expect(detail.data.value).toEqual({ uid: 2 })
    expect(detail.loading.value).toBe(false)
  })

  it('reset 作废在途请求并清空展示状态', async () => {
    const entry = gate()
    const detail = useAdminDetail<Detail, number>(() => entry.promise, '加载用户详情失败')

    const pending = detail.load(9)
    detail.reset()

    expect(detail.data.value).toBeUndefined()
    expect(detail.error.value).toBe('')
    expect(detail.loading.value).toBe(false)

    entry.resolve({ code: '00000', data: { uid: 9 } })
    await pending
    // 换筛选条件后，在途的旧详情不得重新落地
    expect(detail.data.value).toBeUndefined()
  })
})
