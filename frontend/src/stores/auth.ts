import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

const STORAGE_KEY = 'cdk-admin-auth'

interface StoredAuth {
  token?: string
  username: string
  name: string
  expiresAt: string
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref('')
  const username = ref('')
  const name = ref('')
  const expiresAt = ref('')

  // 认证状态由 HttpOnly cookie 维持；前端只用 username + 到期时间判断登录态（不依赖 token）
  const isLoggedIn = computed(() => {
    if (!username.value || !expiresAt.value) return false
    return new Date(expiresAt.value).getTime() > Date.now()
  })

  const restore = () => {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return

    try {
      const saved = JSON.parse(raw) as StoredAuth
      token.value = saved.token || ''
      username.value = saved.username
      name.value = saved.name
      expiresAt.value = saved.expiresAt
      if (!isLoggedIn.value) logout()
    } catch (error) {
      logout()
    }
  }

  const save = (payload: StoredAuth) => {
    token.value = payload.token || ''
    username.value = payload.username
    name.value = payload.name
    expiresAt.value = payload.expiresAt
    localStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
  }

  const logout = () => {
    token.value = ''
    username.value = ''
    name.value = ''
    expiresAt.value = ''
    localStorage.removeItem(STORAGE_KEY)
  }

  return {
    token,
    username,
    name,
    expiresAt,
    isLoggedIn,
    restore,
    save,
    logout,
  }
})
