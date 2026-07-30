import { useAuthStore } from '../stores/auth'

const UNSAFE_METHODS = ['POST', 'PUT', 'PATCH', 'DELETE']

function getCookie(name: string): string {
  const match = document.cookie.match(new RegExp('(?:^|; )' + name.replace(/([.$?*|{}()[\]\\/+^])/g, '\\$1') + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : ''
}

function requestUrl(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.href
  try {
    return String((input as Request).url || '')
  } catch {
    return ''
  }
}

/**
 * 是否应把 401/403 当成「本站登录失效」。
 * 卡台上游 Key 无效/未配置时后端可能透传 401/403，绝不能踢出 admin 会话。
 */
function isLocalSessionAuthFailure(url: string, status: number, body: any): boolean {
  if (status !== 401 && status !== 403) return false

  // 上游卡台 / 网络探测：业务失败，不是 CDK 登录失效
  if (/\/api\/v1\/admin\/cardplatform\//i.test(url)) return false
  if (/\/api\/v1\/admin\/network\//i.test(url)) return false

  const err = String(body?.error || body?.msg || body?.message || '').toLowerCase()
  const code = String(body?.error_code || body?.code || '').toLowerCase()

  // 明确卡台/上游错误码
  if (
    code.includes('cardplatform')
    || code.includes('api_key')
    || err.includes('api key')
    || err.includes('openapi')
    || err.includes('卡台')
    || err.includes('spacexcard')
  ) {
    return false
  }

  // 本站鉴权常见文案
  if (
    err.includes('missing authorization')
    || err.includes('invalid or expired token')
    || err.includes('invalid authorization')
    || err.includes('csrf')
    || code === 'unauthorized'
  ) {
    return true
  }

  // 默认：仅对「非卡台」的 401/403 踢登录；403 CSRF 也算会话保护
  return true
}

export const authFetch = async (input: RequestInfo | URL, init: RequestInit = {}) => {
  const authStore = useAuthStore()
  const headers = new Headers(init.headers || {})
  const url = requestUrl(input)

  // 兼容旧会话：localStorage 里若还有 token 就带 Bearer 头；新会话走 HttpOnly cookie
  if (authStore.token) {
    headers.set('Authorization', `Bearer ${authStore.token}`)
  }

  // 基于 cookie 的写操作需要 CSRF 双提交：把可读的 csrf_token cookie 放进请求头
  const method = (init.method || 'GET').toUpperCase()
  if (UNSAFE_METHODS.includes(method)) {
    const csrf = getCookie('csrf_token')
    if (csrf) headers.set('X-CSRF-Token', csrf)
  }

  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(input, {
    ...init,
    headers,
    credentials: 'include',
  })

  if (response.status === 401 || response.status === 403) {
    let body: any = null
    try {
      body = await response.clone().json()
    } catch {
      body = null
    }
    if (isLocalSessionAuthFailure(url, response.status, body)) {
      authStore.logout()
      if (window.location.pathname.startsWith('/ops')) {
        window.location.href = '/ops/login'
      }
    }
  }

  return response
}

// 主动登出：先请求后端清除 HttpOnly cookie，再清本地状态
export const serverLogout = async () => {
  try {
    const csrf = getCookie('csrf_token')
    const headers = new Headers()
    if (csrf) headers.set('X-CSRF-Token', csrf)
    await fetch('/api/v1/auth/admin/logout', { method: 'POST', headers, credentials: 'include' })
  } catch (error) {
    // 忽略网络错误，本地状态照常清除
  }
  useAuthStore().logout()
}
