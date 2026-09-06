import http from 'node:http'
import type { AdminTokenPayload, Envelope } from '../shared/types'

/** 换取后台 JWT 的结果（与下发给 renderer 的载荷同一形状） */
export type AdminTokenResult = AdminTokenPayload

/**
 * 向后台登录接口换取 JWT。
 *
 * 为什么由主进程做：管理员凭据在桌面应用托管的 .env 里（ADMIN_PASSWORD），
 * 由主进程登录可让凭据完全不进入 renderer，也免去一个重复输入自己设过的密码的登录页。
 *
 * @param adminBaseUrl 后台 API 基址，形如 http://127.0.0.1:9091/admin/api
 * @param timeoutMs 单次请求超时，默认 5 秒
 */
export function loginAsAdmin(
  adminBaseUrl: string,
  username: string,
  password: string,
  timeoutMs = 5000,
): Promise<AdminTokenResult> {
  return new Promise((resolve) => {
    let payload = ''
    try {
      const url = new URL(`${adminBaseUrl.replace(/\/$/, '')}/login`)
      const body = JSON.stringify({ username, password })
      const req = http.request(
        url,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Content-Length': Buffer.byteLength(body),
          },
        },
        (res) => {
          res.setEncoding('utf-8')
          res.on('data', (chunk: string) => {
            // 防御异常大响应，信封只有几百字节
            if (payload.length < 64_000) payload += chunk
          })
          res.on('end', () => {
            let envelope: Envelope<{ token?: string }>
            try {
              envelope = JSON.parse(payload) as Envelope<{ token?: string }>
            } catch {
              resolve({ error: `后台登录响应无法解析（HTTP ${res.statusCode}）` })
              return
            }
            const token = envelope.data?.token
            if (envelope.code === '00000' && typeof token === 'string' && token) {
              resolve({ token })
              return
            }
            resolve({ error: envelope.error || `后台登录失败（code=${envelope.code ?? '未知'}）` })
          })
        },
      )
      req.on('error', (error: Error) => resolve({ error: `无法连接后台接口：${error.message}` }))
      req.setTimeout(timeoutMs, () => {
        req.destroy(new Error('timeout'))
      })
      req.write(body)
      req.end()
    } catch (error) {
      resolve({ error: `后台登录请求构造失败：${String(error)}` })
    }
  })
}

/** 登录所需的端点与凭据快照 */
export interface AdminCredentialSource {
  adminBaseUrl: string
  username: string
  password: string
}

/**
 * 后台 JWT 的持有者与刷新器。
 * 缓存 token，并在被后台拒绝或端点/凭据变化时重新登录。
 */
export class AdminTokenStore {
  private token: string | null = null
  /** 生成代次：使并发中的旧请求结果不会覆盖新结果 */
  private generation = 0
  private pending: Promise<AdminTokenResult> | null = null

  constructor(private readonly source: () => AdminCredentialSource | null) {}

  /** 丢弃缓存（服务重启、配置变更后必须重新登录） */
  invalidate(): void {
    this.token = null
    this.generation++
    this.pending = null
  }

  /**
   * 取得可用 token；命中缓存则不发登录请求。
   * @param forceRefresh 收到鉴权失败时置 true，强制重登一次
   */
  async obtain(forceRefresh = false): Promise<AdminTokenResult> {
    if (!forceRefresh && this.token) return { token: this.token }

    const creds = this.source()
    if (!creds) {
      return { error: '尚未读取到有效配置，无法登录后台' }
    }
    if (!creds.username || !creds.password) {
      return { error: '管理员账号或密码未设置，请在配置页填写后重启服务' }
    }

    // 同一时刻只允许一次登录：401 风暴会打爆 5 次/分钟的登录限流（A0241）
    if (this.pending) return this.pending

    const generation = this.generation
    this.pending = loginAsAdmin(creds.adminBaseUrl, creds.username, creds.password).then((result) => {
      if (result.token && generation === this.generation) {
        this.token = result.token
      }
      if (generation === this.generation) this.pending = null
      return result
    })
    return this.pending
  }
}
