import { describe, expect, it } from 'vitest'
import { createRequestGuard } from './requestGuard'

describe('createRequestGuard', () => {
  it('只认最新一次请求', () => {
    const guard = createRequestGuard()
    const first = guard.begin()
    const second = guard.begin()

    expect(guard.isCurrent(first)).toBe(false)
    expect(guard.isCurrent(second)).toBe(true)
  })

  it('invalidate 作废在途请求', () => {
    const guard = createRequestGuard()
    const id = guard.begin()

    guard.invalidate()

    expect(guard.isCurrent(id)).toBe(false)
  })
})
