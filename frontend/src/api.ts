/**
 * 后台 API 调用 + 管理端 token 持久化（ADR-0010 独立后台 JWT；ADR-0024 token 存 localStorage）。
 *
 * 所有失败都以信封返回，不向视图抛异常；鉴权失败统一清 token 并回登录页。
 */

export interface Envelope<T = unknown> {
  /** 业务码，必填：后台每个分支都经 errcode 信封出口回码，没有无码响应。 */
  code: string
  error?: string
  data?: T
}

/** 列表端点的 data 形状 */
export interface Page<T> {
  list: T[]
  total: number
}

/** 概览统计（用户数 + 评论数） */
export interface Stats {
  users: number
  comments: number
}

/** model.User 的后台视图字段 */
export interface AdminUser {
  uid: number
  email: string
  username: string
  nickname: string
  avatar: string
  created_at: string
}

/** GET /admin/api/users/:uid 的 data：账号资料 + 该用户评论数 */
export interface UserDetail {
  user: AdminUser
  comment_count: number
}

/** 后台评论固定视图：作者只含展示字段，不含 email（ADR-0015） */
export interface AdminComment {
  id: number
  user_id: number
  user?: { username?: string; nickname?: string }
  content: string
  chapter_name?: string
  book_name?: string
  created_at: string
}

/** 操作日志的后台视图字段 */
export interface AdminLog {
  id: number
  user_id: number
  username: string
  method: string
  path: string
  ip: string
  user_agent: string
  response_code: number
  error_code: string
  error_message: string
  created_at: string
}

export interface UserQuery {
  page?: number
  pageSize?: number
  keyword?: string
}

export interface CommentQuery {
  page?: number
  pageSize?: number
  keyword?: string
  bookName?: string
}

export interface LogQuery {
  page?: number
  pageSize?: number
  method?: string
  path?: string
  /** 只看业务码非 00000 的失败请求 */
  failed?: boolean
}

const TOKEN_KEY = 'ebook_admin_token'

/**
 * 管理端令牌：每次读写都现取 localStorage，模块初始化期不碰任何浏览器全局。
 *
 * 视图、单测与非浏览器消费者都会 `import './api'`；把一次 `getItem` 留在导入期，
 * 整个模块在没有 DOM 的环境里当场抛错（本仓 vitest 门禁红在此）。存储不可用时
 * 读作空令牌、写作无效——浏览器里走不到这条分支，只保证导入方不被拖垮。
 */
export const token = {
  get value(): string {
    return tokenStore()?.getItem(TOKEN_KEY) ?? ''
  },
  set(v: string) {
    const store = tokenStore()
    if (!store) return
    try {
      if (v) store.setItem(TOKEN_KEY, v)
      else store.removeItem(TOKEN_KEY)
    } catch {
      // 配额满或被策略禁用时写入会抛：与「没有存储」同等对待，不能让登录/退出
      // 因为一次写入失败变成未处理异常。
    }
  },
}

function tokenStore(): Storage | undefined {
  try {
    return globalThis.localStorage ?? undefined
  } catch {
    // 隐私模式等场景下取 localStorage 本身就会抛：等同于「没有存储」
    return undefined
  }
}

const BASE = '/admin/api'

/**
 * 成功业务码。唯一事实在后端 `model.SuccessCode`；前端只在这一个常量上对齐，
 * 视图与列表模块不再各写一份字面量。
 */
export const SUCCESS_CODE = '00000'

/** 后台引擎的每个鉴权失败分支都回 A0403（`adminAuthFailureCode`），没有第二种码。 */
const AUTH_FAILURE_CODES = new Set(['A0403'])

/** 把查询参数拼成 URL 查询串；undefined 与空串省略，所以「不过滤」要传 undefined 而不是 false。 */
function toQuery(params: Record<string, string | number | boolean | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') continue
    search.set(key, String(value))
  }
  return search.toString()
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'DELETE'
  body?: unknown
  /** false 用于登录：不带令牌，也不因失败触发登录跳转。 */
  auth?: boolean
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<Envelope<T>> {
  const { method = 'GET', body, auth = true } = opts
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (auth && token.value) headers.Authorization = `Bearer ${token.value}`

  let res: Response
  try {
    res = await fetch(BASE + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch (e) {
    return {
      code: 'C0500',
      error: `无法连接后端服务：${e instanceof Error ? e.message : String(e)}`,
    }
  }

  // 业务信封的 HTTP 状态恒为 200；非 2xx 只可能来自前置代理、网关或静态服务。
  // 这类响应的正文常是 HTML，不能把 JSON 解析错误原文当成「后端说」的话展示出去。
  if (!res.ok) {
    return { code: 'C0500', error: `后端服务返回异常状态（HTTP ${res.status}）` }
  }

  // `res.json()` 的结果只是 any：`as Envelope<T>` 不做任何校验。200 + `null`（代理、
  // 静态服务的兜底响应）会在视图里以 TypeError 逃出去，违背「只回信封、不向视图抛异常」
  // 的契约；code 不是字符串时同样无法判定成败。畸形正文按 C0500 交回去。
  let parsed: unknown
  try {
    parsed = await res.json()
  } catch {
    return { code: 'C0500', error: '后端返回了无法解析的响应' }
  }
  if (!isEnvelope<T>(parsed)) {
    return { code: 'C0500', error: '后端返回了无法识别的响应' }
  }
  if (auth && AUTH_FAILURE_CODES.has(parsed.code)) {
    token.set('')
    if (typeof window !== 'undefined') window.location.hash = '#/login'
  }
  return parsed
}

/** 统一信封的最小形状：非空对象且 code 是字符串。 */
function isEnvelope<T>(value: unknown): value is Envelope<T> {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as { code?: unknown }).code === 'string'
  )
}

export function login(username: string, password: string): Promise<Envelope<{ token: string }>> {
  return request('/login', { method: 'POST', auth: false, body: { username, password } })
}

export function fetchStats(): Promise<Envelope<Stats>> {
  return request('/stats')
}

export function fetchUsers(query: UserQuery = {}): Promise<Envelope<Page<AdminUser>>> {
  return request(`/users?${toQuery({ page: query.page, page_size: query.pageSize, keyword: query.keyword })}`)
}

export function fetchUserDetail(uid: number): Promise<Envelope<UserDetail>> {
  return request(`/users/${uid}`)
}

export function fetchComments(query: CommentQuery = {}): Promise<Envelope<Page<AdminComment>>> {
  return request(
    `/comments?${toQuery({
      page: query.page,
      page_size: query.pageSize,
      keyword: query.keyword,
      book_name: query.bookName,
    })}`,
  )
}

/** 删除评论（后台治理，不做归属校验） */
export function deleteComment(id: number): Promise<Envelope<never>> {
  return request(`/comments/${id}`, { method: 'DELETE' })
}

export function fetchLogs(query: LogQuery = {}): Promise<Envelope<Page<AdminLog>>> {
  // failed=false 是「不过滤」，必须显式传 true 才参与
  return request(
    `/logs?${toQuery({
      page: query.page,
      page_size: query.pageSize,
      method: query.method,
      path: query.path,
      failed: query.failed ? true : undefined,
    })}`,
  )
}
