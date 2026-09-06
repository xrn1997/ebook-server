/**
 * 管理后台 API 调用（HTTP 访问 Go sidecar 的 /admin/api/*）。
 *
 * 基址与 JWT 都向主进程索取：端口来自受管配置（写死端口会让用户改完配置后静默失效），
 * 管理员凭据只留在主进程，renderer 拿到的只是可过期的令牌。
 */
import { isAdminAuthFailure, type Envelope } from '../../shared/types'
import * as ipc from './electron'

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

/** model.Comment 的后台视图字段（user 为预加载的完整用户，密码不会序列化） */
export interface AdminComment {
  id: number
  user_id: number
  user?: { username?: string; nickname?: string }
  content: string
  chapter_name?: string
  book_name?: string
  created_at: string
}

/** model.OperationLog 的后台视图字段 */
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

/** 用户列表查询参数（零值/undefined 字段不进查询串） */
export interface UserQuery {
  page?: number
  pageSize?: number
  keyword?: string
}

/** 评论列表查询参数 */
export interface CommentQuery {
  page?: number
  pageSize?: number
  keyword?: string
  bookName?: string
}

/** 日志列表查询参数 */
export interface LogQuery {
  page?: number
  pageSize?: number
  method?: string
  path?: string
  /** 只看业务码非 00000 的失败请求 */
  failed?: boolean
}

/** 把查询参数拼成 URL 查询串；undefined 与空串字段一律省略 */
function toQuery(params: Record<string, string | number | boolean | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') continue
    search.set(key, String(value))
  }
  return search.toString()
}

/** 正在进行的令牌换取：401 风暴会打爆后台 5 次/分钟的登录限流（A0241），故需合流 */
let pendingToken: Promise<{ token: string; error?: string }> | null = null
let cachedToken = ''

/**
 * 取得后台 JWT。
 * @param force 置 true 时忽略缓存，让主进程重新登录一次
 */
async function acquireToken(force: boolean): Promise<{ token: string; error?: string }> {
  if (cachedToken && !force) return { token: cachedToken }
  if (!pendingToken) {
    pendingToken = ipc.getAdminToken(force).then((result) => {
      pendingToken = null
      cachedToken = result.token ?? ''
      return { token: cachedToken, error: result.error }
    })
  }
  return pendingToken
}

/** 丢弃缓存令牌（服务停止或重启后旧 token 不再有意义） */
export function forgetToken(): void {
  cachedToken = ''
}

function failure(message: string): Envelope<never> {
  return { code: 'A0403', error: message }
}

/** 发一次实际请求；网络异常或响应不是 JSON 时返回 null */
async function call<T>(baseUrl: string, path: string, token: string, method: 'GET' | 'DELETE'): Promise<Envelope<T> | null> {
  try {
    const res = await fetch(baseUrl + path, {
      method,
      headers: { Authorization: 'Bearer ' + token },
    })
    return (await res.json()) as Envelope<T>
  } catch {
    return null
  }
}

/**
 * 调用一个后台端点，自动补齐端点地址与令牌，并在令牌不被接受时重登一次。
 * 后端永不返回非 200，所以「是不是失败」只能看信封里的 code。
 */
async function request<T>(path: string, method: 'GET' | 'DELETE' = 'GET'): Promise<Envelope<T>> {
  const info = await ipc.getRuntimeInfo()
  if (!info || !info.endpoints.adminBaseUrl) {
    return failure('无法取得后台端点：主进程尚未读到有效配置')
  }

  const first = await acquireToken(false)
  if (!first.token) {
    return failure(`后台自动登录失败：${first.error ?? '未取得令牌'}`)
  }

  let response = await call<T>(info.endpoints.adminBaseUrl, path, first.token, method)
  if (response && isAdminAuthFailure(response.code)) {
    const retry = await acquireToken(true)
    if (!retry.token) {
      return failure(`后台自动登录失败：${retry.error ?? '未取得令牌'}`)
    }
    response = await call<T>(info.endpoints.adminBaseUrl, path, retry.token, method)
  }

  return response ?? failure('后台响应无法解析')
}

/** 概览统计（用户数 + 评论数） */
export function fetchStats(): Promise<Envelope<Stats>> {
  return request<Stats>('/stats')
}

/** 用户列表（keyword 命中邮箱/用户名/昵称，纯数字兼命中 UID） */
export function fetchUsers(query: UserQuery = {}): Promise<Envelope<Page<AdminUser>>> {
  return request<Page<AdminUser>>(`/users?${toQuery({
    page: query.page,
    page_size: query.pageSize,
    keyword: query.keyword,
  })}`)
}

/** 用户详情（账号资料 + 该用户评论数） */
export function fetchUserDetail(uid: number): Promise<Envelope<UserDetail>> {
  return request<UserDetail>(`/users/${uid}`)
}

/** 评论列表（keyword 匹配内容，bookName 精确过滤） */
export function fetchComments(query: CommentQuery = {}): Promise<Envelope<Page<AdminComment>>> {
  return request<Page<AdminComment>>(`/comments?${toQuery({
    page: query.page,
    page_size: query.pageSize,
    keyword: query.keyword,
    book_name: query.bookName,
  })}`)
}

/** 删除评论（后台治理，不做归属校验） */
export function deleteComment(id: number): Promise<Envelope<never>> {
  return request<never>(`/comments/${id}`, 'DELETE')
}

/** 操作日志列表（method/path/failed 筛选） */
export function fetchLogs(query: LogQuery = {}): Promise<Envelope<Page<AdminLog>>> {
  // failed=false 是「不过滤」，必须显式传 true 才参与
  return request<Page<AdminLog>>(`/logs?${toQuery({
    page: query.page,
    page_size: query.pageSize,
    method: query.method,
    path: query.path,
    failed: query.failed ? 'true' : undefined,
  })}`)
}
