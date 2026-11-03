import { ref } from 'vue'
import { unwrap, type Envelope } from './api'
import { createRequestGuard } from './requestGuard'

/**
 * 单次详情类调用的加载状态（与 useAdminList 同族）。
 *
 * 请求序号与 useAdminList.loadPage 同一套语义：连续点开两次详情时，
 * 先发后回的旧响应整段丢弃，不覆盖已经落地的新结果。
 *
 * 取哪条记录由 `load(arg)` 的入参给出，而不是让调用方的 fetcher 去闭包读一个可变的外层
 * 变量：本函数在第一个 `await` 之前同步调用 fetcher，所以那种写法今天碰巧是对的，
 * 但一旦有人在前面插入 await（防抖、令牌刷新），它就会读到「当前」而不是「请求时」的 id。
 * 刻意不自带 onMounted：视图自己决定何时加载，也让本模块能在无 DOM 环境下直接测。
 */
export function useAdminDetail<T, A>(
  fetcher: (arg: A) => Promise<Envelope<T>>,
  fallbackMessage: string,
) {
  const data = ref<T>()
  const error = ref('')
  const loading = ref(false)

  /**
   * 请求序号：只认最新一次发出的响应（与 useAdminList 共用 requestGuard）。
   * 作废在途请求（reset）也靠它：序号一增，旧响应落地时整段丢弃。
   */
  const requests = createRequestGuard()

  /** 拉取 `arg` 那一条详情；成功时 data 赋值、error 清空，失败时只留一句可直接展示的中文原因 */
  async function load(arg: A): Promise<void> {
    const id = requests.begin()
    loading.value = true
    error.value = ''
    try {
      const result = await unwrap(fetcher(arg), fallbackMessage)
      if (!requests.isCurrent(id)) return
      if (result.ok && result.data !== undefined) {
        data.value = result.data
      } else {
        data.value = undefined
        // 详情必然带载荷：成功却没有 data 是契约漂移（字段改名、整键被 omitempty 掉）。
        // 只判 ok 会把这一格留空——error 空、data 空、loading 落，三链皆假的模板让详情区
        // 整块消失，把后端改动渲染成「没有这条记录」。与 useAdminList 同一判据：契约漂移要报错。
        error.value = result.ok ? '详情数据格式不正确' : result.message
      }
    } catch (e) {
      if (!requests.isCurrent(id)) return
      // api 层已经把网络失败翻译成信封，抛到这里只剩编程错误（同 useAdminList）：
      // 不能把真 bug 伪装成运维问题，只留通用文案、原文进控制台。
      console.error('后台详情加载抛出意外异常', e)
      data.value = undefined
      error.value = fallbackMessage
    } finally {
      if (requests.isCurrent(id)) loading.value = false
    }
  }

  /**
   * 作废详情区的所有在途/已展示状态（换筛选条件或翻页时，旧详情不再适用）。
   *
   * 只清 data 不够：不作废序号的话，在途请求会照常落地，
   * 把上一个筛选条件下的账号重新显示出来。
   */
  function reset(): void {
    requests.invalidate()
    data.value = undefined
    error.value = ''
    loading.value = false
  }

  return { data, error, loading, load, reset }
}
