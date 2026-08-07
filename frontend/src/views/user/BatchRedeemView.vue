<template>
  <div class="min-h-screen py-12">
    <div class="max-w-3xl mx-auto px-6">
      <div class="mb-8 flex items-start justify-between gap-4">
        <div>
          <router-link to="/" class="app-link mb-4 inline-block text-sm">{{ t('common.back') }}</router-link>
          <h1 class="text-3xl font-bold text-ink mb-1">{{ t('batch.title') }}</h1>
          <p class="text-muted text-sm">{{ t('batch.subtitle') }}</p>
        </div>
        <div class="flex items-center gap-3">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </div>

      <RedeemModeTabs />

      <div class="card space-y-4">
        <div class="flex items-start gap-3 pb-3 border-b bd">
          <div
            class="w-10 h-10 rounded-xl flex items-center justify-center shrink-0 text-white text-[10px] font-bold leading-tight text-center"
            style="background: var(--primary)"
          >
            批量<br />兑换
          </div>
          <div>
            <h2 class="text-base font-semibold text-ink">{{ t('batch.panelTitle') }}</h2>
            <p class="text-xs text-muted mt-0.5 leading-relaxed">{{ t('batch.panelHint') }}</p>
          </div>
        </div>

        <!-- input phase -->
        <template v-if="phase === 'input'">
          <div>
            <label class="block text-sm font-medium text-ink mb-1.5">
              {{ t('batch.cdkListLabel', { max: BATCH_MAX_KEYS }) }}
            </label>
            <textarea
              v-model="cdkText"
              rows="6"
              class="input mono text-sm min-h-[120px]"
              :placeholder="t('batch.cdkPlaceholder')"
            />
            <p class="text-xs text-muted mt-1">
              {{ t('batch.cdkRecognized', { n: parsedCdks.length }) }}
              <span v-if="parsedCdks.length > 100" class="text-warn">
                · {{ t('batch.cdkQueueHint') }}
              </span>
            </p>
          </div>

          <ExcelImportBlock
            :session-pool="sessionPool"
            :import-msg="importMsg"
            :importing="importing"
            @pick="onPickFile"
            @clear="clearSessionPool"
          />

          <button
            type="button"
            class="btn-primary w-full"
            :disabled="verifying || parsedCdks.length === 0"
            @click="handleVerifyBatch"
          >
            {{ verifying ? t('batch.verifying') : t('batch.verifyStart') }}
          </button>
          <p v-if="sessionError" class="text-xs text-err">{{ sessionError }}</p>
        </template>

        <!-- run phase -->
        <template v-else>
          <div class="grid grid-cols-4 gap-2 text-center">
            <div class="rounded-lg bg-soft py-2">
              <div class="text-lg font-bold tabular-nums text-ink">{{ stats.total }}</div>
              <div class="text-[10px] text-muted">{{ t('batch.statTotal') }}</div>
            </div>
            <div class="rounded-lg py-2" style="background: color-mix(in srgb, var(--warn, #d97706) 12%, transparent)">
              <div class="text-lg font-bold tabular-nums text-warn">{{ stats.wait }}</div>
              <div class="text-[10px] text-muted">{{ t('batch.statWait') }}</div>
            </div>
            <div class="rounded-lg py-2" style="background: color-mix(in srgb, var(--primary) 12%, transparent)">
              <div class="text-lg font-bold tabular-nums" style="color: var(--primary)">{{ stats.run }}</div>
              <div class="text-[10px] text-muted">{{ t('batch.statRun') }}</div>
            </div>
            <div class="rounded-lg py-2" style="background: color-mix(in srgb, var(--good, #16a34a) 12%, transparent)">
              <div class="text-lg font-bold tabular-nums text-good">
                {{ stats.ok }}<span class="text-muted text-sm font-normal">/{{ stats.fail }}</span>
              </div>
              <div class="text-[10px] text-muted">{{ t('batch.statOkFail') }}</div>
            </div>
          </div>

          <div v-if="!verifying && pendingSessionItems.length > 0" class="rounded-xl border bd p-3">
            <ExcelImportBlock
              :session-pool="sessionPool"
              :import-msg="importMsg"
              :importing="importing"
              @pick="onPickFile"
              @clear="clearSessionPool"
            />
          </div>

          <div v-if="verifying" class="text-sm text-muted text-center py-4">
            {{ t('batch.verifyingKeys') }}
          </div>

          <div
            v-else-if="currentItem"
            class="rounded-xl border p-4 space-y-3"
            style="border-color: color-mix(in srgb, var(--primary) 35%, var(--brd)); background: color-mix(in srgb, var(--primary) 6%, transparent)"
          >
            <div class="flex items-center justify-between gap-2 flex-wrap">
              <div class="text-sm font-medium text-ink">
                {{ t('batch.currentOrdinal', { n: currentOrdinal, total: totalVerified }) }}
                <span class="text-xs font-normal text-muted ml-1">
                  · {{ t('batch.remaining', { n: pendingSessionItems.length }) }}
                </span>
              </div>
              <div class="text-xs font-mono" style="color: var(--primary)">
                {{ currentItem.cardKey }}
                <span v-if="currentItem.planLabel" class="ml-1.5 text-muted">({{ currentItem.planLabel }})</span>
              </div>
            </div>
            <p
              v-if="currentExcelIdx >= 0 && sessionPool[currentExcelIdx]?.email"
              class="text-xs text-muted"
            >
              {{ t('batch.willUseAccount') }}
              <span class="font-mono ml-1 text-ink">{{ sessionPool[currentExcelIdx].email }}</span>
            </p>
            <textarea
              ref="sessionBoxRef"
              v-model="sessionInput"
              rows="5"
              class="input mono text-xs min-h-[100px]"
              :placeholder="t('batch.sessionPlaceholder')"
              :disabled="submitting || autoSubmitting"
              @input="sessionError = ''"
            />
            <p v-if="sessionError" class="text-xs text-err">{{ sessionError }}</p>
            <div class="flex flex-col gap-2">
              <button
                type="button"
                class="btn-primary w-full"
                :disabled="!sessionInput.trim() || submitting || autoSubmitting"
                @click="handleSubmitSession"
              >
                {{ submitting ? t('batch.submitting') : t('batch.submitNext') }}
              </button>
              <button
                v-if="sessionPool.length > 0"
                type="button"
                class="btn-secondary w-full !border-0 text-white"
                style="background: var(--good, #16a34a)"
                :disabled="submitting || autoSubmitting"
                @click="handleAutoSubmitAll"
              >
                {{
                  autoSubmitting
                    ? t('batch.autoSubmitting')
                    : t('batch.autoSubmit', { n: sessionPool.length })
                }}
              </button>
            </div>
            <p class="text-[11px] text-muted text-center">{{ t('batch.pairHint') }}</p>
          </div>

          <div
            v-else-if="allSessionsCollected"
            class="rounded-xl border p-4 text-center space-y-2"
            style="border-color: color-mix(in srgb, var(--good, #16a34a) 35%, var(--brd)); background: color-mix(in srgb, var(--good, #16a34a) 8%, transparent)"
          >
            <p class="text-sm font-medium text-good">
              {{ t('batch.allSubmitted', { n: sessionsDone }) }}
            </p>
            <p class="text-xs text-muted">{{ t('batch.watchProgress') }}</p>
            <button
              v-if="stats.ok > 0"
              type="button"
              class="text-xs px-3 py-1.5 rounded-lg text-white"
              style="background: var(--good, #16a34a)"
              @click="exportBatchSuccess"
            >
              {{ t('batch.exportOk', { n: stats.ok }) }}
            </button>
          </div>

          <div v-else class="text-sm text-muted text-center py-3">
            {{ t('batch.noValidKeys') }}
          </div>

          <div class="rounded-xl border bd overflow-hidden">
            <div class="px-3 py-2 bg-soft text-xs font-medium text-muted">{{ t('batch.liveProgress') }}</div>
            <ul class="divide-y max-h-80 overflow-y-auto" style="border-color: var(--brd)">
              <li
                v-for="(it, idx) in items"
                :key="it.id"
                class="px-3 py-2.5 text-xs space-y-1"
                style="border-color: var(--brd)"
              >
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="text-subtle tabular-nums w-5">{{ idx + 1 }}</span>
                  <span class="font-mono text-ink break-all">{{ it.cardKey }}</span>
                  <span v-if="it.planLabel" class="text-[10px] text-muted">{{ it.planLabel }}</span>
                  <span class="ml-auto px-1.5 py-0.5 rounded font-medium status-pill" :data-st="it.status">
                    {{ statusLabel(it.status) }}
                  </span>
                </div>
                <div
                  v-if="it.progressMsg || it.verifyMsg || it.email || it.redemptionToken"
                  class="pl-7 text-[11px] text-muted space-y-0.5"
                >
                  <div v-if="it.email">{{ t('batch.account') }}：{{ it.email }}</div>
                  <div v-if="it.redemptionToken" class="font-mono truncate">
                    token：{{ it.redemptionToken.slice(0, 18) }}…
                  </div>
                  <div>
                    {{
                      it.status === 'verify_fail' || it.status === 'failed'
                        ? it.error || it.progressMsg || it.verifyMsg
                        : it.progressMsg || it.verifyMsg
                    }}
                  </div>
                </div>
              </li>
            </ul>
          </div>

          <button
            v-if="stats.ok > 0"
            type="button"
            class="btn-secondary w-full"
            style="color: var(--good, #16a34a); border-color: color-mix(in srgb, var(--good, #16a34a) 40%, var(--brd))"
            @click="exportBatchSuccess"
          >
            {{ t('batch.exportOk', { n: stats.ok }) }}
          </button>
          <button type="button" class="btn-secondary w-full" @click="resetAll">
            {{ t('batch.reset') }}
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import LanguageToggle from '../../components/LanguageToggle.vue'
import ThemeToggle from '../../components/ThemeToggle.vue'
import RedeemModeTabs from '../../components/RedeemModeTabs.vue'
import ExcelImportBlock from '../../components/ExcelImportBlock.vue'
import {
  AUTO_SUBMIT_CONCURRENCY,
  BATCH_MAX_KEYS,
  BATCH_POLL_INTERVAL_MS,
  VERIFY_CONCURRENCY,
  accessTokenFromSession,
  checkSessionForCdk,
  exportSuccessWorkbook,
  extractCdkSession,
  mapPool,
  parseCdks,
  parseSessionsFromSheet,
  readWorkbookRows,
  type ImportedSession,
} from '../../lib/batch-session'

const { t } = useI18n({ useScope: 'global' })

type ItemStatus =
  | 'pending_verify'
  | 'verify_ok'
  | 'verify_fail'
  | 'waiting_session'
  | 'submitting'
  | 'processing'
  | 'success'
  | 'failed'

interface BatchItem {
  id: string
  cardKey: string
  planLabel: string
  status: ItemStatus
  verifyMsg: string
  redemptionToken: string
  progressMsg: string
  email: string
  accessToken: string
  gptPassword: string
  emailPassword: string
  error: string
}

const deviceId = (() => {
  const k = 'cdk_device_id'
  let v = localStorage.getItem(k)
  if (!v) {
    v = 'web-' + Math.random().toString(36).slice(2) + Date.now().toString(36)
    localStorage.setItem(k, v)
  }
  return v
})()

const phase = ref<'input' | 'run'>('input')
const cdkText = ref('')
const items = ref<BatchItem[]>([])
const verifying = ref(false)
const sessionInput = ref('')
const sessionError = ref('')
const submitting = ref(false)
const autoSubmitting = ref(false)
const sessionPool = ref<ImportedSession[]>([])
const importMsg = ref('')
const importing = ref(false)
const sessionBoxRef = ref<HTMLTextAreaElement | null>(null)
const lastFilledItemId = ref<string | null>(null)

const pollTargets = ref<Record<string, { token: string; terminal: boolean }>>({})
let pollTimer: ReturnType<typeof setInterval> | null = null
let pollInFlight = false

const parsedCdks = computed(() => parseCdks(cdkText.value))

const verifiedQueue = computed(() =>
  items.value.filter((i) => i.status !== 'pending_verify' && i.status !== 'verify_fail'),
)
const pendingSessionItems = computed(() =>
  items.value.filter((i) => i.status === 'waiting_session' || i.status === 'verify_ok'),
)
const currentItem = computed(() => pendingSessionItems.value[0] ?? null)
const currentExcelIdx = computed(() => {
  if (!currentItem.value) return -1
  return verifiedQueue.value.findIndex((i) => i.id === currentItem.value!.id)
})
const sessionsDone = computed(() =>
  items.value.filter((i) => ['submitting', 'processing', 'success', 'failed'].includes(i.status)).length,
)
const totalVerified = computed(() => verifiedQueue.value.length)
const currentOrdinal = computed(() =>
  currentExcelIdx.value >= 0 ? currentExcelIdx.value + 1 : sessionsDone.value,
)

const stats = computed(() => {
  const s = { total: items.value.length, ok: 0, fail: 0, run: 0, wait: 0 }
  for (const i of items.value) {
    if (i.status === 'success') s.ok++
    else if (i.status === 'failed' || i.status === 'verify_fail') s.fail++
    else if (i.status === 'processing' || i.status === 'submitting') s.run++
    else if (i.status === 'waiting_session' || i.status === 'verify_ok') s.wait++
  }
  return s
})

const allSessionsCollected = computed(
  () =>
    phase.value === 'run' &&
    !verifying.value &&
    totalVerified.value > 0 &&
    pendingSessionItems.value.length === 0,
)

function statusLabel(st: ItemStatus) {
  const map: Record<ItemStatus, string> = {
    pending_verify: t('batch.status.pending_verify'),
    verify_ok: t('batch.status.verify_ok'),
    verify_fail: t('batch.status.verify_fail'),
    waiting_session: t('batch.status.waiting_session'),
    submitting: t('batch.status.submitting'),
    processing: t('batch.status.processing'),
    success: t('batch.status.success'),
    failed: t('batch.status.failed'),
  }
  return map[st] || st
}

function updateItem(id: string, patch: Partial<BatchItem>) {
  items.value = items.value.map((it) => (it.id === id ? { ...it, ...patch } : it))
}

async function api(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers || {})
  headers.set('X-Redemption-Device', deviceId)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const r = await fetch(path, { ...init, headers, credentials: 'include' })
  const text = await r.text()
  let data: any = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = { raw: text }
  }
  return { r, data }
}

const TERMINAL = new Set(['completed', 'declined', 'failed_precharge', 'cancelled', 'failed'])

function isTerminal(st: string) {
  return TERMINAL.has(String(st || '').toLowerCase())
}

function isSuccessStatus(st: string) {
  return String(st || '').toLowerCase() === 'completed'
}

function isFailStatus(st: string) {
  const s = String(st || '').toLowerCase()
  return ['declined', 'failed_precharge', 'cancelled', 'failed'].includes(s)
}

function planLabelOf(data: any): string {
  return String(data?.plan || data?.plan_type || data?.plan_label || data?.data?.plan || '')
}

function stopAllPolls() {
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
  pollInFlight = false
  pollTargets.value = {}
}

async function pollBatch() {
  if (pollInFlight) return
  const entries = Object.entries(pollTargets.value).filter(([, v]) => !v.terminal)
  if (!entries.length) {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
    return
  }
  pollInFlight = true
  try {
    await Promise.all(
      entries.map(async ([id, target]) => {
        try {
          const { r, data } = await api(
            '/api/v1/public/cdk/result?token=' + encodeURIComponent(target.token),
          )
          if (!r.ok) return
          const order = data?.order || data?.data?.order || data?.data || data || {}
          const st = String(order.status || data?.status || data?.data?.status || '')
          const message = String(
            order.message || order.user_message || data?.message || data?.user_message || '',
          )
          if (isSuccessStatus(st)) {
            updateItem(id, { status: 'success', progressMsg: message || '开通完成', error: '' })
            pollTargets.value[id] = { ...target, terminal: true }
          } else if (isFailStatus(st)) {
            updateItem(id, {
              status: 'failed',
              progressMsg: message || '兑换失败',
              error: message || '失败',
            })
            pollTargets.value[id] = { ...target, terminal: true }
          } else {
            updateItem(id, {
              status: 'processing',
              progressMsg: message || (st ? `处理中 · ${st}` : '处理中…'),
              error: '',
            })
          }
        } catch {
          /* transient */
        }
      }),
    )
  } finally {
    pollInFlight = false
  }
}

function startPoll(id: string, token: string) {
  pollTargets.value = { ...pollTargets.value, [id]: { token, terminal: false } }
  if (!pollTimer) {
    void pollBatch()
    pollTimer = setInterval(() => void pollBatch(), BATCH_POLL_INTERVAL_MS)
  }
}

function fillSessionFromPool(excelIdx: number) {
  const pool = sessionPool.value
  if (!pool.length || excelIdx < 0 || excelIdx >= pool.length) {
    sessionInput.value = ''
    return
  }
  sessionInput.value = pool[excelIdx].session
  sessionError.value = ''
}

watch(
  () => currentItem.value?.id,
  async (id) => {
    if (!id || currentExcelIdx.value < 0) return
    if (lastFilledItemId.value === id) return
    lastFilledItemId.value = id
    if (sessionPool.value.length > 0) fillSessionFromPool(currentExcelIdx.value)
    else sessionInput.value = ''
    await nextTick()
    sessionBoxRef.value?.focus()
  },
)

async function onPickFile(file: File | null) {
  if (!file) return
  importing.value = true
  importMsg.value = ''
  try {
    const rows = await readWorkbookRows(file)
    const { sessions, mode, sessionCol, skippedDup } = parseSessionsFromSheet(rows)
    if (!sessions.length) {
      sessionPool.value = []
      importMsg.value =
        sessionCol < 0
          ? '未识别到 Session 列。请确保有 session 列或单元格为完整 Session JSON'
          : '识别到列但无有效 Session（需完整 JSON 含 sessionToken，或五段 JWE）'
      return
    }
    sessionPool.value = sessions
    importMsg.value =
      `已从「${file.name}」识别 ${sessions.length} 条 Session（${mode}）` +
      (skippedDup > 0 ? `，已去重跳过 ${skippedDup} 条` : '') +
      (sessions[0]?.email ? `，如 ${sessions[0].email}` : '')
    lastFilledItemId.value = null
    if (phase.value === 'run' && currentExcelIdx.value >= 0) {
      fillSessionFromPool(currentExcelIdx.value)
    }
  } catch (e) {
    sessionPool.value = []
    importMsg.value = '读取 Excel 失败：' + (e instanceof Error ? e.message : '未知错误')
  } finally {
    importing.value = false
  }
}

function clearSessionPool() {
  sessionPool.value = []
  importMsg.value = ''
  sessionInput.value = ''
}

async function handleVerifyBatch() {
  const keys = parseCdks(cdkText.value)
  if (!keys.length) return
  if (keys.length > BATCH_MAX_KEYS) {
    sessionError.value = `单次最多 ${BATCH_MAX_KEYS} 个卡密`
    return
  }
  verifying.value = true
  sessionError.value = ''
  stopAllPolls()
  lastFilledItemId.value = null
  const list: BatchItem[] = keys.map((k, i) => ({
    id: `${i}-${k}`,
    cardKey: k,
    planLabel: '',
    status: 'pending_verify',
    verifyMsg: '验证中…',
    redemptionToken: '',
    progressMsg: '',
    email: '',
    accessToken: '',
    gptPassword: '',
    emailPassword: '',
    error: '',
  }))
  items.value = list
  phase.value = 'run'
  sessionInput.value = ''

  try {
    const results = await mapPool(list, VERIFY_CONCURRENCY, async (item) => {
      try {
        const { r, data } = await api('/api/v1/public/cdk/preview', {
          method: 'POST',
          body: JSON.stringify({ code: item.cardKey }),
        })
        if (!r.ok) {
          const msg = String(data?.error || data?.msg || data?.message || 'CDK 无效或不可用')
          return {
            ...item,
            status: 'verify_fail' as const,
            verifyMsg: msg,
            error: msg,
          }
        }
        const token = String(
          data?.redemption_token || data?.data?.redemption_token || data?.token || '',
        )
        if (!token) {
          return {
            ...item,
            status: 'verify_fail' as const,
            verifyMsg: '未返回 redemption_token',
            error: '未返回 redemption_token',
          }
        }
        const body = data?.data || data || {}
        return {
          ...item,
          status: 'waiting_session' as const,
          verifyMsg: '有效',
          planLabel: planLabelOf(body),
          redemptionToken: token,
        }
      } catch {
        return {
          ...item,
          status: 'verify_fail' as const,
          verifyMsg: '验证请求失败',
          error: '验证请求失败',
        }
      }
    })
    items.value = results
  } finally {
    verifying.value = false
    lastFilledItemId.value = null
  }
}

async function submitOne(
  item: BatchItem,
  sessionRaw: string,
  imported?: ImportedSession,
): Promise<'ok' | 'fail'> {
  const check = checkSessionForCdk(sessionRaw)
  if (!check.ok) {
    updateItem(item.id, {
      status: 'failed',
      progressMsg: check.error,
      error: check.error,
    })
    return 'fail'
  }
  const session = extractCdkSession(sessionRaw)
  updateItem(item.id, {
    status: 'submitting',
    email: check.email || imported?.email || '',
    accessToken: imported?.accessToken || accessTokenFromSession(sessionRaw),
    gptPassword: imported?.gptPassword || '',
    emailPassword: imported?.emailPassword || '',
    progressMsg: '账号预检…',
    error: '',
  })

  let redemptionToken = item.redemptionToken
  try {
    if (!redemptionToken) {
      const preview = await api('/api/v1/public/cdk/preview', {
        method: 'POST',
        body: JSON.stringify({ code: item.cardKey }),
      })
      if (!preview.r.ok) {
        const msg = String(preview.data?.error || preview.data?.msg || 'CDK 预览失败')
        updateItem(item.id, { status: 'failed', progressMsg: msg, error: msg })
        return 'fail'
      }
      redemptionToken = String(
        preview.data?.redemption_token || preview.data?.data?.redemption_token || '',
      )
      if (!redemptionToken) {
        updateItem(item.id, {
          status: 'failed',
          progressMsg: '未返回 redemption_token',
          error: '未返回 redemption_token',
        })
        return 'fail'
      }
      updateItem(item.id, { redemptionToken })
    }

    const { r, data } = await api('/api/v1/public/cdk/preflight', {
      method: 'POST',
      body: JSON.stringify({
        code: item.cardKey,
        redemption_token: redemptionToken,
        credential: { mode: 'session', session },
      }),
    })
    if (!r.ok || (data && typeof data.code === 'number' && data.code !== 0)) {
      const msg = String(data?.error || data?.msg || data?.message || '预检失败')
      updateItem(item.id, { status: 'failed', progressMsg: msg, error: msg })
      return 'fail'
    }
    const body = data?.data && typeof data.data === 'object' ? data.data : data || {}
    const preflightToken = String(body.preflight_token || data?.preflight_token || '')
    if (!preflightToken) {
      updateItem(item.id, {
        status: 'failed',
        progressMsg: '未返回 preflight_token',
        error: '未返回 preflight_token',
      })
      return 'fail'
    }
    const email = String(body.email || body.account_email || check.email || imported?.email || '')
    updateItem(item.id, { email, progressMsg: '预检通过，提交兑换…' })

    const client_request_id = `batch-${deviceId.slice(0, 8)}-${Date.now()}-${item.id}`
    const redeem = await api('/api/v1/public/cdk/redeem', {
      method: 'POST',
      body: JSON.stringify({
        redemption_token: redemptionToken,
        preflight_token: preflightToken,
        client_request_id,
      }),
    })
    const order =
      redeem.data?.order || redeem.data?.data?.order || redeem.data?.data || redeem.data || {}
    const st = String(order.status || redeem.data?.status || '')
    const message = String(
      order.message || order.user_message || redeem.data?.message || redeem.data?.msg || '',
    )

    if (!redeem.r.ok && redeem.r.status !== 202) {
      updateItem(item.id, {
        status: 'failed',
        progressMsg: message || redeem.data?.error || '兑换被拒绝',
        error: message || redeem.data?.error || '兑换被拒绝',
      })
      return 'fail'
    }

    if (isSuccessStatus(st)) {
      updateItem(item.id, { status: 'success', progressMsg: message || '开通完成', error: '' })
    } else if (isFailStatus(st)) {
      updateItem(item.id, {
        status: 'failed',
        progressMsg: message || '兑换失败',
        error: message || '失败',
      })
      return 'fail'
    } else {
      updateItem(item.id, {
        status: 'processing',
        progressMsg: message || '处理中…',
        error: '',
      })
      startPoll(item.id, redemptionToken)
    }
    return 'ok'
  } catch {
    updateItem(item.id, {
      status: 'failed',
      progressMsg: '网络异常，请用任务查询确认是否已受理',
      error: '网络异常',
    })
    return 'fail'
  }
}

async function handleSubmitSession() {
  if (!currentItem.value || submitting.value || autoSubmitting.value) return
  const check = checkSessionForCdk(sessionInput.value)
  if (!check.ok) {
    sessionError.value = check.error
    return
  }
  sessionError.value = ''
  submitting.value = true
  const item = currentItem.value
  const raw = sessionInput.value
  sessionInput.value = ''
  lastFilledItemId.value = null
  try {
    const imported =
      currentExcelIdx.value >= 0 ? sessionPool.value[currentExcelIdx.value] : undefined
    await submitOne(
      item,
      raw,
      imported?.session.trim() === raw.trim() ? imported : undefined,
    )
  } finally {
    submitting.value = false
  }
}

async function handleAutoSubmitAll() {
  if (autoSubmitting.value || submitting.value || verifying.value) return
  const pool = sessionPool.value
  if (!pool.length) {
    sessionError.value = '请先导入 Excel Session'
    return
  }
  const verifiedSnap = items.value.filter(
    (i) => i.status !== 'pending_verify' && i.status !== 'verify_fail',
  )
  const pendingSnap = items.value.filter(
    (i) => i.status === 'verify_ok' || i.status === 'waiting_session',
  )
  if (!pendingSnap.length) {
    sessionError.value = '没有待提交的卡密'
    return
  }
  autoSubmitting.value = true
  sessionError.value = ''
  const verifiedPositions = new Map(verifiedSnap.map((item, index) => [item.id, index]))
  let cursor = 0
  const workers = Array.from(
    { length: Math.min(AUTO_SUBMIT_CONCURRENCY, pendingSnap.length) },
    async () => {
      while (true) {
        const itemIndex = cursor++
        if (itemIndex >= pendingSnap.length) return
        const item = pendingSnap[itemIndex]
        const excelIdx = verifiedPositions.get(item.id) ?? -1
        const sess = excelIdx >= 0 ? pool[excelIdx] : undefined
        if (!sess) {
          updateItem(item.id, {
            status: 'failed',
            progressMsg: `Excel 只有 ${pool.length} 条 Session，第 ${excelIdx + 1} 张无对应`,
            error: 'Session 不足',
          })
          continue
        }
        await submitOne(item, sess.session, sess)
      }
    },
  )
  await Promise.all(workers)
  sessionInput.value = ''
  autoSubmitting.value = false
}

function exportBatchSuccess() {
  const ok = items.value.filter((i) => i.status === 'success')
  if (!ok.length) {
    sessionError.value = '本批尚无成功记录'
    return
  }
  sessionError.value = ''
  const part = (value: string) => String(value || '').replace(/[\r\n]+/g, ' ').trim()
  exportSuccessWorkbook(
    ok.map((i) => [part(i.email), part(i.gptPassword), part(i.emailPassword), part(i.accessToken)]),
  )
}

function resetAll() {
  stopAllPolls()
  phase.value = 'input'
  items.value = []
  cdkText.value = ''
  sessionInput.value = ''
  sessionError.value = ''
  autoSubmitting.value = false
  lastFilledItemId.value = null
}

onUnmounted(() => stopAllPolls())
</script>

<style scoped>
.text-good { color: var(--good, #16a34a); }
.text-warn { color: var(--warn, #d97706); }
.text-err { color: var(--err, #dc2626); }
.status-pill[data-st='pending_verify'] { background: var(--soft); color: var(--muted); }
.status-pill[data-st='verify_ok'],
.status-pill[data-st='success'] {
  background: color-mix(in srgb, var(--good, #16a34a) 16%, transparent);
  color: var(--good, #16a34a);
}
.status-pill[data-st='verify_fail'],
.status-pill[data-st='failed'] {
  background: color-mix(in srgb, var(--err, #dc2626) 14%, transparent);
  color: var(--err, #dc2626);
}
.status-pill[data-st='waiting_session'] {
  background: color-mix(in srgb, var(--warn, #d97706) 14%, transparent);
  color: var(--warn, #d97706);
}
.status-pill[data-st='submitting'],
.status-pill[data-st='processing'] {
  background: color-mix(in srgb, var(--primary) 14%, transparent);
  color: var(--primary);
}
</style>
