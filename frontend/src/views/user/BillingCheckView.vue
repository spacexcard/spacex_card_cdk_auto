<template>
  <div class="min-h-screen py-12">
    <div class="max-w-3xl mx-auto px-6 space-y-6">
      <div class="flex items-start justify-between gap-4">
        <div>
          <router-link to="/" class="app-link mb-4 inline-block text-sm">返回首页</router-link>
          <h1 class="text-3xl font-bold text-ink">账单查询</h1>
          <p class="text-sm text-muted mt-1">
            支持 <b>卡密</b>（使用兑换时绑定的 session）或直接粘贴 session 查询订阅与账单链接。
          </p>
        </div>
        <div class="flex gap-2">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </div>

      <div class="card space-y-4">
        <div class="flex gap-2">
          <button
            type="button"
            class="btn-secondary !py-1.5"
            :class="{ 'ring-2 ring-offset-1': mode === 'cdk' }"
            style="--tw-ring-color: var(--primary)"
            @click="mode = 'cdk'"
          >卡密查询</button>
          <button
            type="button"
            class="btn-secondary !py-1.5"
            :class="{ 'ring-2 ring-offset-1': mode === 'session' }"
            style="--tw-ring-color: var(--primary)"
            @click="mode = 'session'"
          >Session 查询</button>
        </div>

        <template v-if="mode === 'cdk'">
          <div class="rounded-xl bg-soft p-4 text-sm text-muted">
            输入兑换用的 CDK。系统使用兑换预检时保存的 session 去拉取账单（无需再贴 token）。
            若从未用 session 完成过兑换，请改用「Session 查询」。
          </div>
          <div class="form-group">
            <label>CDK 卡密</label>
            <input
              v-model="cdkCode"
              class="input mono"
              placeholder="SXC-XXXX-XXXX-XXXX-XXXX"
              @keyup.enter="query"
            />
          </div>
        </template>

        <template v-else>
          <div class="rounded-xl bg-soft p-4 text-sm text-muted">
            打开
            <a class="app-link underline" href="https://chatgpt.com/api/auth/session" target="_blank" rel="noopener">
              chatgpt.com/api/auth/session
            </a>
            ，复制<strong>完整 JSON</strong>（须含 sessionToken）。已禁用纯 Access Token。
          </div>
          <div class="form-group">
            <label>完整 Session JSON</label>
            <textarea
              v-model="tokenInput"
              class="input h-36 font-mono text-xs"
              placeholder='{"user":{...},"accessToken":"eyJ...","sessionToken":"eyJ..."}'
            />
          </div>
        </template>

        <div v-if="error" class="alert alert-error">{{ error }}</div>
        <button
          class="btn-primary w-full"
          :disabled="loading || !canSubmit"
          @click="query"
        >
          {{ loading ? '查询中…' : '查询订阅与账单' }}
        </button>
      </div>

      <div v-if="summary" class="card space-y-3">
        <div class="flex items-center justify-between">
          <h2 class="text-xl font-semibold text-ink">订阅摘要</h2>
          <el-tag v-if="authSource" size="small" type="info">{{ authSource === 'cdk' ? '来自卡密绑定' : '来自 session' }}</el-tag>
        </div>
        <div class="grid sm:grid-cols-2 gap-2 text-sm">
          <div class="flex justify-between gap-2 border-b bd py-2">
            <span class="text-muted">方案</span>
            <b class="text-ink">{{ planLabel }}</b>
          </div>
          <div class="flex justify-between gap-2 border-b bd py-2">
            <span class="text-muted">有效订阅</span>
            <b>{{ summary.has_active_subscription ? '是' : '否' }}</b>
          </div>
          <div class="flex justify-between gap-2 border-b bd py-2">
            <span class="text-muted">自动续费</span>
            <b>{{ summary.will_renew == null ? '—' : (summary.will_renew ? '开启' : '已关闭') }}</b>
          </div>
          <div class="flex justify-between gap-2 border-b bd py-2">
            <span class="text-muted">计费</span>
            <b class="mono">{{ summary.billing_currency || '—' }} {{ summary.billing_period || '' }}</b>
          </div>
          <div class="flex justify-between gap-2 border-b bd py-2 sm:col-span-2">
            <span class="text-muted">到期 / 续费</span>
            <b class="mono text-sm">{{ summary.active_until || summary.expires_at || summary.renews_at || '—' }}</b>
          </div>
        </div>
      </div>

      <div v-if="invoices" class="card space-y-3">
        <h2 class="text-xl font-semibold text-ink">付款账单（{{ invoices.length }}）</h2>
        <p v-if="!invoices.length" class="text-sm text-muted">无付款账单记录。</p>
        <div v-for="inv in invoices" :key="inv.id || inv.number" class="rounded-xl bg-soft p-4 text-sm space-y-2">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <b class="mono">{{ formatAmount(inv.total, inv.currency) }}</b>
            <span class="text-muted">{{ inv.paid ? '已付' : (inv.status || '未付') }} · {{ formatTs(inv.created) }}</span>
          </div>
          <div v-if="inv.description" class="text-subtle text-xs">{{ inv.description }}</div>
          <div class="flex gap-3">
            <a v-if="inv.hosted_invoice_url" class="app-link" :href="inv.hosted_invoice_url" target="_blank" rel="noopener">账单链接</a>
            <a v-if="inv.invoice_pdf" class="app-link" :href="inv.invoice_pdf" target="_blank" rel="noopener">PDF</a>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import LanguageToggle from '../../components/LanguageToggle.vue'
import ThemeToggle from '../../components/ThemeToggle.vue'

const mode = ref<'cdk' | 'session'>('cdk')
const cdkCode = ref('')
const tokenInput = ref('')
const loading = ref(false)
const error = ref('')
const summary = ref<any>(null)
const invoices = ref<any[] | null>(null)
const authSource = ref('')

const canSubmit = computed(() =>
  mode.value === 'cdk' ? !!cdkCode.value.trim() : !!tokenInput.value.trim(),
)

const planLabel = computed(() => {
  const s = summary.value || {}
  const raw = (s.plan_type || s.subscription_plan || '').toString()
  if (!raw || raw === 'free') return '免费 / 未知'
  return raw.replace('chatgpt', 'ChatGPT ').replace(/_/g, ' ')
})

function formatAmount(total: any, currency: any) {
  const n = Number(total)
  if (!Number.isFinite(n)) return '—'
  const cur = (currency || 'usd').toString().toUpperCase()
  return `${(n / 100).toFixed(2)} ${cur}`
}
function formatTs(v: any) {
  const n = Number(v)
  if (!Number.isFinite(n) || n <= 0) return '—'
  try {
    return new Date(n * 1000).toLocaleString()
  } catch {
    return String(v)
  }
}

async function query() {
  error.value = ''
  summary.value = null
  invoices.value = null
  authSource.value = ''
  loading.value = true
  try {
    const body =
      mode.value === 'cdk'
        ? { cdk_code: cdkCode.value.trim() }
        : { token_input: tokenInput.value }
    const r = await fetch('/api/v1/public/billing/check', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      error.value = d.error || '查询失败'
      return
    }
    summary.value = d.summary || {}
    invoices.value = d.invoices || []
    authSource.value = d.auth_source || ''
  } catch (e: any) {
    error.value = e?.message || '网络错误'
  } finally {
    loading.value = false
  }
}
</script>
