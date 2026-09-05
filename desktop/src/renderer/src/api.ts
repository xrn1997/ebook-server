/**
 * 管理后台 API 调用（通过 HTTP 访问 Go sidecar 的 /admin/api/* 端点）。
 */

const ADMIN_BASE = 'http://localhost:9091/admin/api'

let adminToken = localStorage.getItem('admin_token') || ''

export function setToken(t: string): void {
  adminToken = t
  if (t) localStorage.setItem('admin_token', t)
  else localStorage.removeItem('admin_token')
}

export function getToken(): string {
  return adminToken
}

async function request(path: string, { method = 'GET', body }: { method?: string; body?: unknown } = {}) {
  const res = await fetch(ADMIN_BASE + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(adminToken ? { Authorization: 'Bearer ' + adminToken } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  return res.json()
}

export async function login(username: string, password: string) {
  return request('/login', { method: 'POST', body: { username, password } })
}

export async function fetchStats() {
  return request('/stats')
}

export async function fetchUsers(page = 1, pageSize = 20) {
  return request(`/users?page=${page}&page_size=${pageSize}`)
}

export async function fetchComments(page = 1, pageSize = 20) {
  return request(`/comments?page=${page}&page_size=${pageSize}`)
}

export async function fetchLogs(page = 1, pageSize = 20) {
  return request(`/logs?page=${page}&page_size=${pageSize}`)
}
