import { describe, it, expect, afterEach } from 'vitest'
import http from 'node:http'
import { AddressInfo } from 'node:net'
import { AdminTokenStore, loginAsAdmin } from '../admin-auth'
import { isAdminAuthFailure } from '../../shared/types'

/**
 * 这些测试跑在真实的 HTTP 服务器上（端口 0 由系统分配），不打桩：
 * 自动登录这条链路此前完全没接通，最需要验证的是真实信封能否被解开。
 */

interface StubServer {
  baseUrl: string
  loginCalls: number
  close: () => Promise<void>
}

const servers: StubServer[] = []

/** 起一个假后台：/login 按 handler 给定的信封应答，并累计被调用次数 */
async function startStubAdmin(handler: (calls: number) => { status: number; body: string }): Promise<StubServer> {
  const state: StubServer = { baseUrl: '', loginCalls: 0, close: async () => {} }
  const server = http.createServer((req, res) => {
    if (req.url?.startsWith('/admin/api/login')) {
      state.loginCalls += 1
      const reply = handler(state.loginCalls)
      res.writeHead(reply.status, { 'Content-Type': 'application/json' })
      res.end(reply.body)
      return
    }
    res.writeHead(404)
    res.end()
  })
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
  const port = (server.address() as AddressInfo).port
  state.baseUrl = `http://127.0.0.1:${port}/admin/api`
  state.close = () => new Promise<void>((resolve) => server.close(() => resolve()))
  servers.push(state)
  return state
}

afterEach(async () => {
  while (servers.length > 0) {
    await servers.pop()?.close()
  }
})

const okEnvelope = JSON.stringify({ code: '00000', error: '', data: { token: 'jwt-abc' } })

describe('loginAsAdmin', () => {
  it('成功时从统一信封里取出 token', async () => {
    const stub = await startStubAdmin(() => ({ status: 200, body: okEnvelope }))
    const result = await loginAsAdmin(stub.baseUrl, 'admin', 'pw')
    expect(result.token).toBe('jwt-abc')
    expect(result.error).toBeUndefined()
  })

  it('凭据错误时给出后台写的文案（后端 HTTP 恒 200，只能看 code）', async () => {
    const stub = await startStubAdmin(() => ({
      status: 200,
      body: JSON.stringify({ code: 'A0403', error: '账号或密码错误', data: null }),
    }))
    const result = await loginAsAdmin(stub.baseUrl, 'admin', 'wrong')
    expect(result.token).toBeUndefined()
    expect(result.error).toBe('账号或密码错误')
  })

  it('响应不是 JSON 时不抛异常', async () => {
    const stub = await startStubAdmin(() => ({ status: 200, body: '<html>proxy</html>' }))
    const result = await loginAsAdmin(stub.baseUrl, 'admin', 'pw')
    expect(result.error).toContain('无法解析')
  })

  it('后台没起来时给出连接失败而不是挂住', async () => {
    const result = await loginAsAdmin('http://127.0.0.1:1/admin/api', 'admin', 'pw', 1000)
    expect(result.error).toContain('无法连接后台接口')
  })
})

describe('AdminTokenStore', () => {
  function storeFor(stub: StubServer, creds: { username: string; password: string } = { username: 'admin', password: 'pw' }) {
    return new AdminTokenStore(() => ({
      adminBaseUrl: stub.baseUrl,
      username: creds.username,
      password: creds.password,
    }))
  }

  it('命中缓存时不再登录（后台登录限流 5 次/分钟）', async () => {
    const stub = await startStubAdmin(() => ({ status: 200, body: okEnvelope }))
    const store = storeFor(stub)

    await store.obtain()
    await store.obtain()

    expect(stub.loginCalls).toBe(1)
  })

  it('invalidate 后重新登录', async () => {
    const stub = await startStubAdmin(() => ({ status: 200, body: okEnvelope }))
    const store = storeFor(stub)

    await store.obtain()
    store.invalidate()
    await store.obtain()

    expect(stub.loginCalls).toBe(2)
  })

  it('并发索取只发一次登录', async () => {
    const stub = await startStubAdmin(() => ({ status: 200, body: okEnvelope }))
    const store = storeFor(stub)

    const results = await Promise.all([store.obtain(), store.obtain(), store.obtain()])

    expect(stub.loginCalls).toBe(1)
    expect(results.every((r) => r.token === 'jwt-abc')).toBe(true)
  })

  it('管理员密码为空时不去撞登录接口，直接给出可操作的提示', async () => {
    const stub = await startStubAdmin(() => ({ status: 200, body: okEnvelope }))
    const store = storeFor(stub, { username: 'admin', password: '' })

    const result = await store.obtain()

    expect(stub.loginCalls).toBe(0)
    expect(result.error).toContain('管理员账号或密码未设置')
  })

  it('无有效配置时不去请求', async () => {
    const store = new AdminTokenStore(() => null)
    const result = await store.obtain()
    expect(result.error).toContain('尚未读取到有效配置')
  })
})

describe('isAdminAuthFailure', () => {
  it('识别无令牌/令牌无效/登录过期三种业务码', () => {
    expect(isAdminAuthFailure('A0403')).toBe(true)
    expect(isAdminAuthFailure('A0240')).toBe(true)
    expect(isAdminAuthFailure('A0230')).toBe(true)
  })

  it('成功码与业务错误不算鉴权失败（避免对普通错误反复重登）', () => {
    expect(isAdminAuthFailure('00000')).toBe(false)
    expect(isAdminAuthFailure('A0241')).toBe(false)
    expect(isAdminAuthFailure(undefined)).toBe(false)
  })
})
