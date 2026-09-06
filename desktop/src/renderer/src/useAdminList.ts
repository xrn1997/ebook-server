import { computed, ref, type Ref } from 'vue'
import type { Envelope } from '../../shared/types'
import type { Page } from './api'

/** 每页条数固定 20（与后台 API 缺省一致）：管理列表没有自定义页长的真实需求 */
export const PAGE_SIZE = 20

/** 列表加载参数：页码由本模块驱动，筛选条件由视图闭包捕获 */
export interface ListParams {
  page: number
  pageSize: number
}

/**
 * 管理列表视图共用的加载与分页状态。
 * 用户/评论/日志三张表除了行怎么渲染之外形状完全一致，重复的
 * 「调一次接口 → 判信封 → 存 rows / 记 error / 翻页」放这里。
 *
 * 刻意不自带 onMounted：视图自己决定何时首屏加载，也让本模块能在无 DOM 环境下直接测。
 */
export function useAdminList<T>(load: (params: ListParams) => Promise<Envelope<Page<T>>>) {
  const rows = ref<T[]>([]) as Ref<T[]>
  const total = ref(0)
  const page = ref(1)
  const error = ref('')
  const loading = ref(false)

  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
  const hasPrev = computed(() => page.value > 1)
  const hasNext = computed(() => page.value < pageCount.value)

  /** 拉取一页；成功时清空错误，失败时只留一句可直接展示的中文原因 */
  async function loadPage(target: number): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const resp = await load({ page: target, pageSize: PAGE_SIZE })
      if (resp.code === '00000') {
        rows.value = resp.data?.list ?? []
        total.value = resp.data?.total ?? 0
        page.value = target
      } else {
        rows.value = []
        error.value = resp.error || '加载失败'
      }
    } catch (e) {
      rows.value = []
      error.value = `无法连接后端服务：${e instanceof Error ? e.message : String(e)}`
    } finally {
      loading.value = false
    }
  }

  /** 首屏与「筛选条件变化后回到第一页」共用 */
  async function reload(): Promise<void> {
    await loadPage(1)
  }

  async function gotoPage(target: number): Promise<void> {
    await loadPage(Math.min(Math.max(1, target), pageCount.value))
  }

  const nextPage = (): Promise<void> => gotoPage(page.value + 1)
  const prevPage = (): Promise<void> => gotoPage(page.value - 1)

  return { rows, total, page, pageCount, hasPrev, hasNext, error, loading, reload, gotoPage, nextPage, prevPage }
}
