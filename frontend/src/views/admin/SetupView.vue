<template>
  <div class="relative min-h-screen flex items-center justify-center px-6 py-10">
    <div class="w-full max-w-lg animate-slideInUp">
      <div class="card space-y-5 !p-7">
        <div class="text-center">
          <h1 class="text-2xl font-bold text-ink">首次安装</h1>
          <p class="mt-2 text-sm text-muted">
            仅可执行一次。生成管理员后会自动登录，并请你保存密码。
          </p>
        </div>

        <div v-if="installed && !done" class="alert alert-success">
          已安装完成。请前往登录。
          <div class="mt-3">
            <router-link class="btn-primary" to="/ops/login">去登录</router-link>
          </div>
        </div>

        <template v-else-if="!done">
          <div class="form-group">
            <label>Setup Token（安装钥匙，不是登录密码）</label>
            <input
              v-model="setupToken"
              class="input font-mono"
              type="text"
              autocomplete="off"
              spellcheck="false"
              placeholder="服务器启动日志 / SETUP_TOKEN.once 里的一串"
            />
          </div>

          <div class="form-group">
            <label>管理员用户名</label>
            <input v-model="username" class="input" maxlength="32" autocomplete="username" />
          </div>

          <div class="flex gap-2">
            <button class="btn-primary flex-1" :disabled="loading" @click="bootstrap('generate')">
              {{ loading && mode === 'generate' ? '生成中…' : '一键生成并进入后台' }}
            </button>
            <button class="btn-secondary" :disabled="loading" type="button" @click="showManual = !showManual">
              自设密码
            </button>
          </div>

          <div v-if="showManual" class="space-y-3 border-t bd pt-4">
            <div class="form-group">
              <label>密码（≥12，字母+数字）</label>
              <input v-model="password" class="input" type="password" autocomplete="new-password" />
            </div>
            <div class="form-group">
              <label>确认密码</label>
              <input v-model="confirm" class="input" type="password" autocomplete="new-password" />
            </div>
            <button class="btn-primary w-full" :disabled="loading" type="button" @click="bootstrap('manual')">
              创建并进入后台
            </button>
          </div>

          <div v-if="error" class="alert alert-error">{{ error }}</div>
        </template>

        <template v-else>
          <div class="alert alert-success">
            安装成功，已自动登录。请立刻复制保存下面的密码（只显示一次）。
          </div>
          <div class="form-group">
            <label>用户名</label>
            <input class="input font-mono text-lg" :value="resultUser" readonly @focus="($event.target as HTMLInputElement).select()" />
          </div>
          <div class="form-group">
            <label>登录密码（请复制）</label>
            <input class="input font-mono text-lg tracking-wide" :value="resultPass" readonly @focus="($event.target as HTMLInputElement).select()" />
          </div>
          <button class="btn-secondary w-full" type="button" @click="copyCreds">
            {{ copied ? '已复制' : '复制账号密码' }}
          </button>
          <label class="flex items-center gap-2 text-sm text-muted">
            <input v-model="saved" type="checkbox" /> 我已安全保存密码
          </label>
          <button
            class="btn-primary w-full"
            type="button"
            :disabled="!saved"
            @click="enterOps"
          >
            进入运营后台
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../../stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const installed = ref(false)
const done = ref(false)
const loading = ref(false)
const mode = ref<'generate' | 'manual'>('generate')
const showManual = ref(false)
const setupToken = ref(sessionStorage.getItem('setup_token') || '')
const username = ref('admin')
const password = ref('')
const confirm = ref('')
const error = ref('')
const resultUser = ref('')
const resultPass = ref('')
const saved = ref(false)
const copied = ref(false)

onMounted(async () => {
  try {
    const r = await fetch('/api/v1/setup/status', { credentials: 'include' })
    const d = await r.json()
    installed.value = !!d.installed
  } catch {
    /* ignore */
  }
})

async function bootstrap(m: 'generate' | 'manual') {
  error.value = ''
  mode.value = m
  if (!setupToken.value.trim()) {
    error.value = '请填写 Setup Token（安装钥匙，不是登录密码）'
    return
  }
  sessionStorage.setItem('setup_token', setupToken.value.trim())
  loading.value = true
  try {
    const body: Record<string, string> = {
      mode: m,
      username: username.value.trim() || 'admin',
    }
    if (m === 'manual') {
      body.password = password.value
      body.confirm_password = confirm.value
    }
    const r = await fetch('/api/v1/setup/bootstrap', {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        'X-Setup-Token': setupToken.value.trim(),
      },
      body: JSON.stringify(body),
    })
    const d = await r.json()
    if (r.status === 410 || d.error === 'already_installed') {
      installed.value = true
      error.value = '已安装，无法再次执行。请直接登录。'
      return
    }
    if (!r.ok) {
      // 把后端错误翻成中文，避免「密码」类文案误解
      const map: Record<string, string> = {
        'invalid setup token': 'Setup Token 错误（请填服务器给的安装钥匙）',
        'setup not allowed from this address': '当前 IP 不允许安装（联系管理员放开 SETUP_ALLOW_CIDRS）',
        'password too weak (min 12, not common, not equal to username)': '密码太弱（至少 12 位，字母+数字）',
        'passwords do not match': '两次密码不一致',
        'password must include letters and digits': '密码需同时包含字母和数字',
      }
      error.value = map[d.error] || d.error || '安装失败'
      return
    }
    resultUser.value = d.username
    resultPass.value = d.password
    done.value = true
    sessionStorage.removeItem('setup_token')
    // 自动登录：后端已下发 cookie；本地会话态同步
    if (d.username) {
      authStore.save({
        username: d.username,
        name: d.name || d.username,
        expiresAt: d.expires_at,
        token: d.token,
      })
    }
    // 暂存到 session，登录页也可预填（兜底）
    sessionStorage.setItem('post_setup_user', d.username || '')
    sessionStorage.setItem('post_setup_pass', d.password || '')
  } catch (e: any) {
    error.value = e?.message || '网络错误'
  } finally {
    loading.value = false
  }
}

async function copyCreds() {
  const text = `username: ${resultUser.value}\npassword: ${resultPass.value}`
  try {
    await navigator.clipboard.writeText(text)
    copied.value = true
  } catch {
    /* ignore */
  }
}

function enterOps() {
  router.push('/ops')
}
</script>
