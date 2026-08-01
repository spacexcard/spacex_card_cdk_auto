<template>
  <div class="min-h-screen py-12">
    <div class="max-w-3xl mx-auto px-6">
      <div class="mb-8 flex items-start justify-between gap-4">
        <div>
          <router-link to="/" class="app-link mb-4 inline-block text-sm">{{ t('common.back') }}</router-link>
          <h1 class="text-3xl font-bold text-ink mb-1">CDK 兑换</h1>
          <p class="text-muted text-sm">经本站转发卡台公开接口：preview → preflight → redeem → 查询结果</p>
        </div>
        <div class="flex items-center gap-3">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </div>

      <!-- redeem-flow v2: no public fee reference -->
      <!-- steps -->
      <div class="card mb-6">
        <div class="flex gap-2 text-sm flex-wrap">
          <span v-for="(s, i) in steps" :key="s" class="pill" :class="step === i + 1 ? 'pill-info' : ''">{{ i + 1 }}. {{ s }}</span>
        </div>
      </div>

      <!-- 1 preview -->
      <div v-show="step === 1" class="card space-y-4">
        <h2 class="text-xl font-bold text-ink">输入 CDK</h2>
        <input v-model="code" class="input mono" placeholder="SXC-XXXX-XXXX-XXXX-XXXX" @keyup.enter="doPreview" />
        <div v-if="error" class="alert alert-error">{{ error }}</div>
        <div v-if="previewInfo" class="rounded-xl bg-soft p-4 text-sm space-y-1">
          <div>套餐：<b>{{ previewInfo.plan || previewInfo.plan_type || '—' }}</b></div>
          <div class="text-muted">{{ previewInfo.message || '码有效，可继续' }}</div>
        </div>
        <button class="btn-primary w-full" :disabled="busy" @click="doPreview">{{ busy ? '校验中…' : '预览 / 下一步' }}</button>
      </div>

      <!-- 2 preflight -->
      <div v-show="step === 2" class="card space-y-4">
        <h2 class="text-xl font-bold text-ink">ChatGPT 凭证</h2>
        <div class="flex gap-2">
          <button type="button" class="btn-secondary !py-1" :class="{ 'ring-2': credMode === 'session' }" @click="credMode = 'session'">Session</button>
          <button type="button" class="btn-secondary !py-1" :class="{ 'ring-2': credMode === 'mailbox' }" @click="credMode = 'mailbox'">邮箱</button>
        </div>
        <template v-if="credMode === 'session'">
          <p class="text-sm text-muted">打开
            <a class="app-link" href="https://chatgpt.com/api/auth/session" target="_blank" rel="noopener">chatgpt.com/api/auth/session</a>
            复制<strong>完整 JSON</strong>（必须含 <code>sessionToken</code>）。已禁用纯 Access Token。
          </p>
          <textarea v-model="sessionRaw" class="input h-36 font-mono text-xs" placeholder='{"user":{...},"accessToken":"eyJ...","sessionToken":"eyJ...五段JWE..."}' />
        </template>
        <template v-else>
          <input v-model="email" class="input" placeholder="email@outlook.com" />
          <input v-model="password" class="input" type="password" placeholder="邮箱密码" />
        </template>
        <div v-if="error" class="alert alert-error">{{ error }}</div>
        <div class="flex gap-3">
          <button class="btn-secondary flex-1" @click="step = 1">上一步</button>
          <button class="btn-primary flex-1" :disabled="busy" @click="doPreflight">{{ busy ? '预检中…' : '预检' }}</button>
        </div>
      </div>

      <!-- 3 redeem -->
      <div v-show="step === 3" class="card space-y-4">
        <h2 class="text-xl font-bold text-ink">确认兑换</h2>
        <p class="text-sm text-muted">将提交兑换请求；结果不确定或 review 时请勿重复提交，请轮询结果。</p>
        <div v-if="error" class="alert alert-error">{{ error }}</div>
        <div class="flex gap-3">
          <button class="btn-secondary flex-1" @click="step = 2">上一步</button>
          <button class="btn-primary flex-1" :disabled="busy" @click="doRedeem">{{ busy ? '提交中…' : '兑换' }}</button>
        </div>
      </div>

      <!-- 4 result -->
      <div v-show="step === 4" class="card space-y-4">
        <h2 class="text-xl font-bold text-ink">兑换进度</h2>

        <div class="flex flex-wrap items-center gap-2">
          <el-tag :type="statusTagType(resultStatus)" size="large">{{ resultStatus || '—' }}</el-tag>
          <span v-if="resultStage" class="text-sm text-muted mono">阶段 {{ resultStage }}</span>
          <span v-if="polling" class="text-sm text-muted">
            <span class="inline-block animate-pulse">●</span> 实时轮询中（约 3s）
          </span>
          <span v-else-if="isTerminal(resultStatus)" class="text-sm" :class="resultStatus === 'completed' ? 'text-good' : 'text-muted'">
            已结束
          </span>
        </div>

        <div v-if="resultMessage" class="rounded-xl bg-soft p-3 text-sm text-ink">
          {{ resultMessage }}
        </div>

        <!-- 进度步骤条（由 stage / events 推导） -->
        <div class="grid grid-cols-4 gap-2 text-center text-xs">
          <div
            v-for="p in progressSteps"
            :key="p.key"
            class="rounded-lg border px-2 py-2"
            :class="p.active ? 'border-primary bg-primary/10 text-ink font-semibold' : 'border-brd text-muted'"
          >
            {{ p.label }}
          </div>
        </div>

        <!-- 时间线明细（卡台 events） -->
        <div v-if="timeline.length" class="space-y-0">
          <div class="text-sm font-medium text-ink mb-2">处理明细</div>
          <ol class="space-y-3 border-l-2 pl-4" style="border-color: var(--brd)">
            <li v-for="(ev, idx) in timeline" :key="ev.id || idx" class="relative">
              <span
                class="absolute -left-[1.35rem] top-1 h-2.5 w-2.5 rounded-full"
                :style="{ background: eventDot(ev.category) }"
              />
              <div class="flex flex-wrap items-baseline gap-2">
                <b class="text-sm text-ink">{{ stepLabel(ev.step) }}</b>
                <el-tag size="small" :type="categoryTag(ev.category)">{{ ev.category || 'pending' }}</el-tag>
                <span class="text-xs text-subtle">{{ fmtTime(ev.created_at) }}</span>
              </div>
              <p class="text-sm text-muted mt-0.5">
                {{ ev.public_message || ev.to_status || '—' }}
              </p>
            </li>
          </ol>
        </div>
        <div v-else-if="polling" class="text-sm text-muted">
          已提交，等待卡台返回步骤明细…
        </div>

        <details class="text-xs text-muted">
          <summary class="cursor-pointer select-none">原始响应（调试）</summary>
          <pre class="rounded-xl bg-soft p-4 overflow-auto max-h-48 mt-2">{{ resultPretty }}</pre>
        </details>

        <div v-if="error" class="alert alert-error">{{ error }}</div>
        <div v-if="isTerminal(resultStatus) && resultStatus !== 'completed'" class="alert alert-error">
          兑换未成功。若状态为 review/pending 请勿重复提交；可联系发码方或稍后用同一设备再查结果。
        </div>
        <div v-if="resultStatus === 'completed'" class="alert alert-success">开通完成，请到 ChatGPT 账号确认套餐。</div>
        <button class="btn-secondary" @click="resetAll">再兑一张</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import LanguageToggle from '../../components/LanguageToggle.vue'
import ThemeToggle from '../../components/ThemeToggle.vue'

const { t } = useI18n({ useScope: 'global' })
const steps = ['预览', '凭证', '兑换', '结果']
const step = ref(1)
const busy = ref(false)
const error = ref('')
const code = ref('')
const previewInfo = ref<any>(null)
const redemptionToken = ref('')
const preflightToken = ref('')
const credMode = ref<'session' | 'mailbox'>('session')
const sessionRaw = ref('')
const email = ref('')
const password = ref('')
const resultStatus = ref('')
const resultStage = ref('')
const resultMessage = ref('')
const resultBody = ref<any>(null)
const timeline = ref<any[]>([])
const polling = ref(false)
let pollTimer: any = null

const PROGRESS_KEY = 'cdk_redeem_progress_v1'

const deviceId = (() => {
  const k = 'cdk_device_id'
  let v = localStorage.getItem(k)
  if (!v) {
    v = 'web-' + Math.random().toString(36).slice(2) + Date.now().toString(36)
    localStorage.setItem(k, v)
  }
  return v
})()

function saveProgress() {
  try {
    const payload = {
      step: step.value,
      code: code.value,
      redemptionToken: redemptionToken.value,
      preflightToken: preflightToken.value,
      resultStatus: resultStatus.value,
      resultStage: resultStage.value,
      resultMessage: resultMessage.value,
      resultBody: resultBody.value,
      timeline: timeline.value,
      savedAt: Date.now(),
    }
    sessionStorage.setItem(PROGRESS_KEY, JSON.stringify(payload))
  } catch {
    /* ignore quota */
  }
}

function loadProgress(): boolean {
  try {
    const raw = sessionStorage.getItem(PROGRESS_KEY)
    if (!raw) return false
    const p = JSON.parse(raw)
    if (!p || typeof p !== 'object') return false
    // 超过 7 天丢弃
    if (p.savedAt && Date.now() - Number(p.savedAt) > 7 * 24 * 3600 * 1000) {
      sessionStorage.removeItem(PROGRESS_KEY)
      return false
    }
    if (p.code) code.value = String(p.code)
    if (p.redemptionToken) redemptionToken.value = String(p.redemptionToken)
    if (p.preflightToken) preflightToken.value = String(p.preflightToken)
    if (p.resultStatus) resultStatus.value = String(p.resultStatus)
    if (p.resultStage) resultStage.value = String(p.resultStage)
    if (p.resultMessage) resultMessage.value = String(p.resultMessage)
    if (p.resultBody) resultBody.value = p.resultBody
    if (Array.isArray(p.timeline)) timeline.value = p.timeline
    const s = Number(p.step) || 1
    // 有 token 即可恢复到结果页
    if (redemptionToken.value && s >= 3) {
      step.value = 4
      return true
    }
    if (s >= 1 && s <= 4) {
      step.value = s
      return s > 1
    }
  } catch {
    /* ignore */
  }
  return false
}

function clearProgress() {
  try {
    sessionStorage.removeItem(PROGRESS_KEY)
  } catch {
    /* ignore */
  }
}

watch(
  [step, code, redemptionToken, preflightToken, resultStatus, resultStage, resultMessage, resultBody, timeline],
  () => saveProgress(),
  { deep: true },
)

const resultPretty = computed(() => JSON.stringify(resultBody.value, null, 2))

const TERMINAL = new Set(['completed', 'declined', 'failed_precharge', 'cancelled', 'failed'])

function isTerminal(st: string) {
  return TERMINAL.has(String(st || '').toLowerCase())
}

function statusTagType(st: string) {
  const s = String(st || '').toLowerCase()
  if (s === 'completed') return 'success'
  if (['declined', 'failed_precharge', 'cancelled', 'failed'].includes(s)) return 'danger'
  if (['review', 'pending', 'card_open_review', 'card_recharge_review'].includes(s)) return 'warning'
  return 'info'
}

function categoryTag(c: string) {
  if (c === 'success' || c === 'completed') return 'success'
  if (c === 'failed' || c === 'error') return 'danger'
  if (c === 'warning') return 'warning'
  return 'info'
}

function eventDot(c: string) {
  if (c === 'success' || c === 'completed') return 'var(--good, #16a34a)'
  if (c === 'failed' || c === 'error') return 'var(--err, #dc2626)'
  if (c === 'warning') return 'var(--warn, #d97706)'
  return 'var(--primary, #2563eb)'
}

function stepLabel(stepKey: string) {
  const map: Record<string, string> = {
    queued: '排队受理',
    credential_check: '凭证校验',
    pricing: '计价',
    checkout: '开卡/绑卡',
    payment: '支付扣款',
    subscription: '订阅生效',
    invoice: '账单',
    renewal: '续费处理',
    reconcile: '对账确认',
    completed: '完成',
  }
  return map[stepKey] || stepKey || '处理中'
}

function fmtTime(v: any) {
  if (!v) return ''
  try {
    const d = new Date(v)
    if (Number.isNaN(d.getTime())) return String(v)
    return d.toLocaleString()
  } catch {
    return String(v)
  }
}

/** 粗粒度进度条：受理 → 开卡/资金 → 支付 → 开通 */
const progressSteps = computed(() => {
  const keys = [
    { key: 'accept', label: '受理' },
    { key: 'card', label: '开卡/资金' },
    { key: 'pay', label: '支付' },
    { key: 'done', label: '开通' },
  ]
  const st = String(resultStatus.value || '').toLowerCase()
  const stage = String(resultStage.value || '').toLowerCase()
  let idx = 0
  if (st === 'completed') idx = 3
  else if (['declined', 'failed_precharge', 'cancelled', 'failed'].includes(st)) {
    // 停在失败前最远一步
    if (stage.includes('pay') || stage.includes('checkout') || stage.includes('subscription')) idx = 2
    else if (stage.includes('card') || stage.includes('fund')) idx = 1
    else idx = 0
  } else if (stage.includes('subscription') || stage.includes('paid') || stage.includes('invoice')) idx = 2
  else if (stage.includes('dispatch') || stage.includes('payment') || stage.includes('checkout') || stage.includes('spend')) idx = 2
  else if (stage.includes('card') || stage.includes('fund') || stage.includes('await')) idx = 1
  else if (timeline.value.some((e) => ['payment', 'subscription', 'checkout'].includes(e.step))) idx = 2
  else if (timeline.value.length) idx = 1
  return keys.map((k, i) => ({ ...k, active: i <= idx }))
})

function applyResultPayload(data: any) {
  resultBody.value = data
  // 卡台公开结构：{ order: {status,stage,message}, events: [] }
  // 兼容顶层扁平 / data 包裹
  const order = data?.order || data?.data?.order || data?.data || data || {}
  const st =
    order.status ||
    data?.status ||
    data?.data?.status ||
    ''
  const stage = order.stage || data?.stage || data?.data?.stage || ''
  const message =
    order.message ||
    order.user_message ||
    data?.message ||
    data?.user_message ||
    data?.data?.message ||
    ''
  resultStatus.value = st
  resultStage.value = stage
  resultMessage.value = message

  let events = data?.events || data?.data?.events || order.events || []
  if (!Array.isArray(events)) events = []
  timeline.value = events.slice().sort((a: any, b: any) => {
    const ta = new Date(a.created_at || 0).getTime()
    const tb = new Date(b.created_at || 0).getTime()
    return ta - tb
  })
}

async function api(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers || {})
  headers.set('X-Redemption-Device', deviceId)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const r = await fetch(path, { ...init, headers, credentials: 'include' })
  const text = await r.text()
  let data: any = null
  try { data = text ? JSON.parse(text) : null } catch { data = { raw: text } }
  return { r, data }
}

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})

/** 完整 Session：必须含 sessionToken（JWE）；禁止纯 AT */
function extractSession(raw: string): string {
  const s = raw.trim()
  if (!s) return ''
  // 裸 JWE session-token
  if (!s.startsWith('{') && s.split('.').length >= 5) return s
  // 三段 JWT = 纯 AT → 拒绝
  if (!s.startsWith('{') && s.startsWith('eyJ') && s.split('.').length === 3) return ''
  if (s.startsWith('{')) {
    try {
      const o = JSON.parse(s)
      const st = String(o.sessionToken || o.session_token || o.token?.sessionToken || '').trim()
      if (!st) return ''
      // 回传完整 JSON，便于预检绑定 session
      return s
    } catch {
      return ''
    }
  }
  return s.length > 40 ? s : ''
}

async function tryResumeByCode(cdk: string): Promise<boolean> {
  const { r, data } = await api(
    '/api/v1/public/cdk/result-by-code?code=' + encodeURIComponent(cdk),
  )
  if (!r.ok) return false
  const tok = data?.redemption_token || data?.data?.redemption_token || ''
  if (tok) redemptionToken.value = tok
  applyResultPayload(data)
  if (!resultStatus.value) resultStatus.value = data?.status || data?.order?.status || 'pending'
  step.value = 4
  startPoll()
  return true
}

async function doPreview() {
  error.value = ''
  previewInfo.value = null
  if (!code.value.trim()) {
    error.value = '请输入 CDK'
    return
  }
  busy.value = true
  try {
    const cdk = code.value.trim()
    const { r, data } = await api('/api/v1/public/cdk/preview', {
      method: 'POST',
      body: JSON.stringify({ code: cdk }),
    })
    if (!r.ok) {
      const msg = data?.error || data?.msg || data?.message || 'CDK 无效或不可用'
      // 已兑换：尝试用本站绑定恢复进度，而不是卡在第一步
      if (/已兑换|已使用|used|redeemed|consumed|已消耗/i.test(String(msg))) {
        const ok = await tryResumeByCode(cdk)
        if (ok) {
          error.value = ''
          return
        }
      }
      error.value = msg
      return
    }
    // 兼容多种返回结构
    redemptionToken.value = data.redemption_token || data.data?.redemption_token || data.token || ''
    previewInfo.value = data.data || data
    if (!redemptionToken.value) {
      // 有的实现把 token 放在顶层其它字段
      error.value = '未返回 redemption_token，请检查卡台 Base 配置'
      return
    }
    step.value = 2
  } finally {
    busy.value = false
  }
}

async function doPreflight() {
  error.value = ''
  busy.value = true
  try {
    let credential: any
    if (credMode.value === 'session') {
      const session = extractSession(sessionRaw.value)
      if (!session) {
        error.value = '请粘贴完整 Session JSON（必须含 sessionToken），不能只用 Access Token'
        return
      }
      credential = { mode: 'session', session }
    } else {
      if (!email.value || !password.value) {
        error.value = '请填写邮箱与密码'
        return
      }
      credential = { mode: 'mailbox', email: email.value.trim(), password: password.value }
    }
    const { r, data } = await api('/api/v1/public/cdk/preflight', {
      method: 'POST',
      body: JSON.stringify({
        code: code.value.trim(),
        redemption_token: redemptionToken.value,
        credential,
      }),
    })
    if (!r.ok) {
      error.value = data?.error || data?.msg || data?.message || '预检失败'
      return
    }
    preflightToken.value = data.preflight_token || data.data?.preflight_token || ''
    if (!preflightToken.value) {
      error.value = '未返回 preflight_token'
      return
    }
    step.value = 3
  } finally {
    busy.value = false
  }
}

async function doRedeem() {
  error.value = ''
  busy.value = true
  try {
    const client_request_id = 'web-' + deviceId.slice(0, 8) + '-' + Date.now()
    const { r, data } = await api('/api/v1/public/cdk/redeem', {
      method: 'POST',
      body: JSON.stringify({
        redemption_token: redemptionToken.value,
        preflight_token: preflightToken.value,
        client_request_id,
      }),
    })
    applyResultPayload(data)
    if (!r.ok && r.status !== 202) {
      error.value = data?.error || data?.msg || data?.message || '兑换被拒绝'
    }
    if (!resultStatus.value) {
      resultStatus.value = r.ok || r.status === 202 ? 'queued' : 'error'
    }
    step.value = 4
    startPoll()
  } finally {
    busy.value = false
  }
}

function startPoll() {
  if (pollTimer) clearInterval(pollTimer)
  if (!redemptionToken.value && !code.value.trim()) {
    polling.value = false
    return
  }
  polling.value = true
  const tick = async () => {
    try {
      let r: Response
      let data: any
      if (redemptionToken.value) {
        ;({ r, data } = await api(
          '/api/v1/public/cdk/result?token=' + encodeURIComponent(redemptionToken.value),
        ))
      } else {
        ;({ r, data } = await api(
          '/api/v1/public/cdk/result-by-code?code=' + encodeURIComponent(code.value.trim()),
        ))
        if (r.ok && data?.redemption_token) {
          redemptionToken.value = data.redemption_token
        }
      }
      if (r.ok) {
        applyResultPayload(data)
        saveProgress()
        if (isTerminal(resultStatus.value)) {
          polling.value = false
          if (pollTimer) clearInterval(pollTimer)
        }
      }
    } catch {
      /* ignore transient network */
    }
  }
  tick()
  pollTimer = setInterval(tick, 3000)
}

onMounted(() => {
  if (loadProgress()) {
    if (step.value === 4 && (redemptionToken.value || code.value)) {
      startPoll()
    }
  }
})

function resetAll() {
  if (pollTimer) clearInterval(pollTimer)
  clearProgress()
  step.value = 1
  error.value = ''
  code.value = ''
  previewInfo.value = null
  redemptionToken.value = ''
  preflightToken.value = ''
  resultBody.value = null
  resultStatus.value = ''
  resultStage.value = ''
  resultMessage.value = ''
  timeline.value = []
  polling.value = false
}
</script>

<style scoped>
.text-good { color: var(--good, #16a34a); }
.border-primary { border-color: var(--primary) !important; }
.bg-primary\/10 { background: color-mix(in srgb, var(--primary) 12%, transparent); }
.border-brd { border-color: var(--brd); }
</style>
