import { useAuthStore } from '../stores/auth'

const UNSAFE_METHODS = ['POST', 'PUT', 'PATCH', 'DELETE']

function getCookie(name: string): string {
  const match = document.cookie.match(new RegExp('(?:^|; )' + name.replace(/([.$?*|{}()[\]\\/+^])/g, '\\$1') + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : ''
}

export const authFetch = async (input: RequestInfo | URL, init: RequestInit = {}) => {
  const authStore = useAuthStore()
  const headers = new Headers(init.headers || {})

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
    authStore.logout()
    if (window.location.pathname.startsWith('/ops')) {
      window.location.href = '/ops/login'
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
