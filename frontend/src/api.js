// 后台 API 调用 + 管理端 token 持久化（ADR-0009：独立于公开 JWT）。
const TOKEN_KEY = 'ebook_admin_token'

export const token = {
  value: localStorage.getItem(TOKEN_KEY) || '',
  set(v) {
    this.value = v
    if (v) localStorage.setItem(TOKEN_KEY, v)
    else localStorage.removeItem(TOKEN_KEY)
  },
}

const base = '/admin/api'

export async function api(path, { method = 'GET', body } = {}) {
  const res = await fetch(base + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token.value ? { Authorization: 'Bearer ' + token.value } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  return res.json() // 统一信封 {code,error,data}
}

export async function login(username, password) {
  return api('/login', { method: 'POST', body: { username, password } })
}

export async function fetchStats() {
  return api('/stats')
}

export async function fetchUsers(page = 1, pageSize = 20) {
  return api(`/users?page=${page}&page_size=${pageSize}`)
}

export async function fetchComments(page = 1, pageSize = 20) {
  return api(`/comments?page=${page}&page_size=${pageSize}`)
}

export async function fetchLogs(page = 1, pageSize = 20) {
  return api(`/logs?page=${page}&page_size=${pageSize}`)
}