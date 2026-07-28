<template>
  <div class="relative min-h-screen flex items-center justify-center px-6 py-10">
    <div class="absolute right-5 top-5 flex items-center gap-3">
      <LanguageToggle />
      <ThemeToggle />
    </div>
    <div class="w-full max-w-md animate-slideInUp">
      <div class="card space-y-6 !p-7">
        <div class="text-center">
          <router-link to="/" class="text-sm app-link">{{ t('common.backHome') }}</router-link>
          <h1 class="mt-4 text-3xl font-bold text-ink">{{ t('login.title') }}</h1>
          <p class="mt-2 text-sm text-muted">{{ t('login.subtitle') }}</p>
        </div>

        <div class="form-group">
          <label>{{ t('login.username') }}</label>
          <input v-model="form.username" class="input" :placeholder="t('login.usernamePlaceholder')" autocomplete="username" @keyup.enter="submit" />
        </div>

        <div class="form-group">
          <label>{{ t('login.password') }}</label>
          <input v-model="form.password" class="input" type="password" :placeholder="t('login.passwordPlaceholder')" autocomplete="current-password" @keyup.enter="submit" />
        </div>

        <div v-if="errorMessage" class="alert alert-error">
          {{ errorMessage }}
        </div>

        <button class="btn-primary w-full disabled:opacity-50" :disabled="loading" @click="submit">
          <span v-if="!loading">{{ t('login.loginBtn') }}</span>
          <span v-else class="flex items-center justify-center gap-2"><span class="spinner"></span>{{ t('login.loggingIn') }}</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '../../stores/auth'
import ThemeToggle from '../../components/ThemeToggle.vue'
import LanguageToggle from '../../components/LanguageToggle.vue'

const { t } = useI18n({ useScope: 'global' })
const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const form = reactive({
  username: sessionStorage.getItem('post_setup_user') || '',
  password: sessionStorage.getItem('post_setup_pass') || '',
})

const loading = ref(false)
const errorMessage = ref('')

// 安装向导写过临时凭据时预填一次后清掉，避免长期留在 sessionStorage
if (form.username || form.password) {
  sessionStorage.removeItem('post_setup_user')
  sessionStorage.removeItem('post_setup_pass')
}

const submit = async () => {
  loading.value = true
  errorMessage.value = ''

  try {
    const payload = {
      username: form.username.trim(),
      password: form.password.trim(),
    }
    const response = await fetch('/api/v1/auth/admin/login', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
    const data = await response.json()

    if (!response.ok) {
      errorMessage.value = data.error || t('login.errLoginFailed')
      return
    }

    // 认证靠 HttpOnly cookie；本地只存展示用元数据
    authStore.save({
      username: data.username,
      name: data.name,
      expiresAt: data.expires_at,
      token: data.token,
    })

    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/ops'
    router.push(redirect)
  } catch (error) {
    errorMessage.value = t('login.errNetwork')
  }

  loading.value = false
}
</script>
