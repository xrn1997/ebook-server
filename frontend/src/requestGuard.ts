/**
 * 只认最新一次请求的序号守卫。
 *
 * 列表翻页与详情切换都会出现「先发后回」：旧响应落地时必须整段丢弃，
 * 否则会用上一套筛选条件的结果覆盖当前页面。两个 hook 共用这一份实现，
 * 避免各自维护一份容易漂移的 `requestId` 比较。
 *
 * `invalidate()` 用于作废在途请求：序号一增，所有已发出的请求都随之过期。
 */
export interface RequestGuard {
  /** 开始一次新请求，返回它的序号。 */
  begin(): number
  /** 该序号对应的请求是否仍是最新一次。 */
  isCurrent(id: number): boolean
  /** 作废所有在途请求（不发起新请求）。 */
  invalidate(): void
}

export function createRequestGuard(): RequestGuard {
  let current = 0
  return {
    begin: () => ++current,
    isCurrent: (id) => id === current,
    invalidate: () => {
      current++
    },
  }
}
