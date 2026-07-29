<template>
  <div class="pb-2 space-y-5">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <h1 class="text-3xl font-bold text-ink">CDK 卡密</h1>
        <p class="text-sm text-muted mt-2">
          卡台 Open API 发码 · 服务费实时计价 · 完整码仅在成功响应中返回（请立即复制/导出）
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <router-link to="/ops/integration" class="btn-secondary">卡台配置 / 出口 IP</router-link>
        <el-button :loading="loadingMeta" @click="refreshAll">刷新价格与列表</el-button>
      </div>
    </div>

    <!-- 状态条 -->
    <div class="card !py-3 flex flex-wrap items-center gap-3 text-sm">
      <el-tag :type="configured ? 'success' : 'danger'" size="small">
        {{ configured ? 'API 已配置' : '未配置 Key' }}
      </el-tag>
      <span v-if="egressIp" class="text-muted">
        出口 IP <b class="mono text-ink">{{ egressIp }}</b>
        <el-button link type="primary" @click="copyText(egressIp)">复制</el-button>
      </span>
      <span class="text-subtle">价格 {{ priceSource }} · v{{ pricingVersion ?? '—' }}</span>
      <span v-if="balanceText" class="text-muted">可消费余额 <b class="mono">{{ balanceText }}</b></span>
      <el-button v-if="!configured" type="warning" size="small" @click="$router.push('/ops/integration')">
        去配置
      </el-button>
    </div>

    <div v-if="metaError" class="alert alert-error">{{ metaError }}</div>

    <!-- 价格卡片：可点选套餐 -->
    <div class="grid gap-3 sm:grid-cols-3">
      <button
        v-for="p in planCards"
        :key="p.key"
        type="button"
        class="card text-left transition plan-card"
        :class="{ 'plan-card--on': form.plan === p.key, 'opacity-60': !p.enabled }"
        :disabled="!p.enabled"
        @click="selectPlan(p.key)"
      >
        <div class="flex items-center justify-between gap-2">
          <span class="font-semibold text-ink">{{ p.label }}</span>
          <el-tag size="small" :type="p.enabled ? 'success' : 'info'">{{ p.enabled ? '可选' : '暂停' }}</el-tag>
        </div>
        <div class="mt-2 text-2xl font-bold mono text-ink">${{ formatUsd(p.service_fee_usd) }}</div>
        <div class="text-xs text-muted mt-1">服务费 / 张（USD）</div>
        <div v-if="form.plan === p.key" class="mt-2 text-xs app-link">已选 · 预计 ${{ estimatedTotal }}</div>
      </button>
    </div>

    <div class="grid gap-6 xl:grid-cols-[400px_minmax(0,1fr)]">
      <!-- 发码 -->
      <section class="card space-y-4">
        <h2 class="text-xl font-semibold text-ink">购买并生成</h2>
        <p class="text-sm text-muted">
          <code>POST /openapi/v1/gpt-direct/cdks</code>
        </p>

        <div class="form-group">
          <label>数量（1–50）</label>
          <div class="flex items-center gap-2">
            <el-button :disabled="form.count <= 1" @click="form.count = Math.max(1, form.count - 1)">−</el-button>
            <input v-model.number="form.count" type="number" min="1" max="50" class="input text-center mono" />
            <el-button :disabled="form.count >= 50" @click="form.count = Math.min(50, form.count + 1)">+</el-button>
            <el-button-group>
              <el-button size="small" @click="form.count = 1">1</el-button>
              <el-button size="small" @click="form.count = 5">5</el-button>
              <el-button size="small" @click="form.count = 10">10</el-button>
            </el-button-group>
          </div>
        </div>

        <el-checkbox v-model="form.funding_confirmed" class="!items-start">
          <span class="text-sm text-muted leading-snug">
            确认 <b class="text-ink">funding_confirmed</b>：兑换时开卡/充值/订阅实付由本账户承担；
            服务费 <b class="mono">${{ estimatedTotal }}</b>
            （{{ form.count }} × ${{ feeOf(form.plan) }}）将从卡台余额扣除。
          </span>
        </el-checkbox>

        <div v-if="issueError" class="alert alert-error">{{ issueError }}</div>
        <div v-if="issueOk" class="alert alert-success">{{ issueOk }}</div>

        <el-button
          type="primary"
          class="w-full"
          size="large"
          :loading="issuing"
          :disabled="!canIssue"
          @click="issue"
        >
          {{ issuing ? '购买中…' : `确认购买 ${form.count} 张 ${form.plan}` }}
        </el-button>
        <p v-if="!configured" class="text-xs" style="color: var(--err)">请先在「卡台配置」填写 Base 与 sk_</p>
        <p v-else-if="!form.funding_confirmed" class="text-xs text-muted">请勾选资金确认后发码</p>

        <div v-if="recentCodes.length" class="rounded-xl bg-soft p-4 space-y-3 border" style="border-color: var(--good)">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div>
              <div class="text-sm font-medium" style="color: var(--good)">完整码（本批 {{ recentCodes.length }} 张）</div>
              <div class="text-xs text-muted mt-0.5">
                卡台明文只返回一次；本页已缓存在浏览器 sessionStorage，刷新后仍可复制。
                <span v-if="recentMeta">套餐 {{ recentMeta.plan }} · {{ recentMeta.atLabel }}</span>
              </div>
            </div>
            <div class="flex flex-wrap gap-1">
              <el-button size="small" type="success" @click="copyAll">全部复制</el-button>
              <el-button size="small" @click="downloadCodes">导出 .txt</el-button>
              <el-button size="small" text type="danger" @click="clearRecent">清除缓存</el-button>
            </div>
          </div>

          <textarea
            class="input mono text-sm !min-h-[120px] w-full"
            readonly
            :value="recentCodes.join('\n')"
            @focus="($event.target as HTMLTextAreaElement).select()"
          />

          <div class="space-y-2">
            <div
              v-for="(c, idx) in recentCodes"
              :key="`${idx}-${c}`"
              class="flex items-start gap-2 rounded-lg px-2 py-1.5"
              style="background: var(--surface)"
            >
              <div class="flex-1 min-w-0">
                <div class="font-mono text-sm break-all select-all" style="color: var(--good); user-select: all">{{ c }}</div>
                <div class="text-xs text-muted mt-0.5">
                  长度 {{ c.length }}
                  <span v-if="!isFullCode(c)" class="ml-1" style="color: var(--err)">（疑似非完整码，请勿当正式卡密）</span>
                </div>
              </div>
              <el-button size="small" type="primary" plain @click="copyText(c)">复制</el-button>
            </div>
          </div>
        </div>
      </section>

      <!-- 列表 -->
      <section class="space-y-3">
        <div class="card flex flex-wrap items-center justify-between gap-3 !py-3">
          <div>
            <h2 class="text-lg font-semibold text-ink">卡台 CDK 列表</h2>
            <p class="text-xs text-muted">仅前缀与状态（完整码不会二次返回）· 共 {{ total }} 条</p>
          </div>
          <el-button :loading="loadingList" @click="loadList">刷新</el-button>
        </div>
        <div v-if="listError" class="alert alert-error">{{ listError }}</div>
        <div class="card overflow-hidden !p-0">
          <el-table :data="rows" v-loading="loadingList" size="small" stripe empty-text="暂无数据">
            <el-table-column prop="id" label="ID" width="80" />
            <el-table-column prop="code_prefix" label="前缀（非完整码）" min-width="140">
              <template #default="{ row }">
                <span class="mono break-all">{{ row.code_prefix || '—' }}</span>
                <span v-if="row.code_prefix" class="text-xs text-subtle ml-1">{{ String(row.code_prefix).length }}字</span>
              </template>
            </el-table-column>
            <el-table-column prop="plan" label="套餐" width="100" />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag size="small" :type="statusType(row.status)">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="服务费" width="90">
              <template #default="{ row }">
                <span class="mono">${{ ((row.fee_amount_minor || 0) / 100).toFixed(2) }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="created_at" label="时间" min-width="140" />
          </el-table>
        </div>
        <div class="flex items-center justify-between text-sm text-muted">
          <span>第 {{ page }} 页</span>
          <div class="flex gap-2">
            <el-button size="small" :disabled="page <= 1" @click="page--; loadList()">上一页</el-button>
            <el-button size="small" :disabled="page * pageSize >= total" @click="page++; loadList()">下一页</el-button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { authFetch } from '../../lib/api'
import { dialog } from '../../lib/dialog'
import { copyToClipboard } from '../../lib/clipboard'

const RECENT_KEY = 'cdk_recent_issued_v1'
/** 卡台完整码形如 GPTD-xxxxxxxxxxxx-xxxxxxxxxxxx-xxxxxxxxxxxx（约 43 字符） */
const FULL_CODE_MIN_LEN = 20

const plans = ref<Record<string, any>>({})
const pricingVersion = ref<number | null>(null)
const priceSource = ref('—')
const balanceText = ref('')
const egressIp = ref('')
const metaError = ref('')
const loadingMeta = ref(false)
const configured = ref(false)

const form = reactive({
  plan: 'plus',
  count: 1,
  funding_confirmed: false,
})
const issuing = ref(false)
const issueError = ref('')
const issueOk = ref('')
const recentCodes = ref<string[]>([])
const recentMeta = ref<{ plan: string; atLabel: string } | null>(null)

const rows = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loadingList = ref(false)
const listError = ref('')

const planCards = computed(() => {
  const order = ['plus', 'pro_5x', 'pro_20x']
  return order.map((k) => {
    const p = plans.value[k] || {}
    return {
      key: k,
      label: p.label || k,
      enabled: p.enabled !== false,
      service_fee_usd: p.service_fee_usd ?? feeDefault(k),
      serviceFeeUsdMinor: p.serviceFeeUsdMinor,
    }
  })
})

const canIssue = computed(() =>
  configured.value && form.funding_confirmed && form.count >= 1 && form.count <= 50 && !issuing.value,
)

function feeDefault(k: string) {
  return k === 'plus' ? 1 : k === 'pro_5x' ? 5 : 10
}
function feeOf(k: string) {
  const p = plans.value[k]
  if (p?.service_fee_usd != null) return Number(p.service_fee_usd).toFixed(2)
  if (p?.serviceFeeUsdMinor != null) return (p.serviceFeeUsdMinor / 100).toFixed(2)
  return feeDefault(k).toFixed(2)
}
function formatUsd(v: any) {
  const n = Number(v)
  return Number.isFinite(n) ? n.toFixed(2) : '—'
}
const estimatedTotal = computed(() => {
  const unit = Number(feeOf(form.plan))
  const c = Math.max(1, Math.min(50, form.count || 1))
  return (unit * c).toFixed(2)
})

function selectPlan(k: string) {
  form.plan = k
}
function statusType(s: string) {
  if (s === 'unused') return 'success'
  if (s === 'consumed') return 'info'
  if (s === 'frozen') return 'warning'
  if (s === 'disabled') return 'danger'
  return ''
}

function isFullCode(code: string) {
  const c = String(code || '').trim()
  // 完整码至少 20 字符；前缀一般 ≤14
  return c.length >= FULL_CODE_MIN_LEN && c.includes('-')
}

function extractFullCode(item: any): string {
  if (!item || typeof item !== 'object') return ''
  const candidates = [item.code, item.full_code, item.cdk_code, item.value]
  for (const raw of candidates) {
    const s = String(raw || '').trim()
    if (isFullCode(s)) return s
  }
  // 绝不把 code_prefix 当完整码
  return ''
}

function persistRecent(codes: string[], plan: string) {
  const payload = { codes, plan, at: Date.now() }
  try {
    sessionStorage.setItem(RECENT_KEY, JSON.stringify(payload))
  } catch {
    /* ignore quota */
  }
  recentMeta.value = {
    plan,
    atLabel: new Date(payload.at).toLocaleString(),
  }
}

function loadPersistedRecent() {
  try {
    const raw = sessionStorage.getItem(RECENT_KEY)
    if (!raw) return
    const o = JSON.parse(raw)
    const codes = Array.isArray(o?.codes) ? o.codes.map((x: any) => String(x || '').trim()).filter(Boolean) : []
    if (!codes.length) return
    recentCodes.value = codes
    recentMeta.value = {
      plan: String(o.plan || '—'),
      atLabel: o.at ? new Date(o.at).toLocaleString() : '—',
    }
  } catch {
    /* ignore */
  }
}

function clearRecent() {
  recentCodes.value = []
  recentMeta.value = null
  try {
    sessionStorage.removeItem(RECENT_KEY)
  } catch {
    /* ignore */
  }
  dialog.toast('已清除本批完整码缓存', 'info')
}

async function copyText(t: string) {
  const ok = await copyToClipboard(t)
  dialog.toast(ok ? '已复制' : '复制失败，请在文本框中全选手动复制', ok ? 'ok' : 'err')
}

async function loadMeta() {
  loadingMeta.value = true
  metaError.value = ''
  try {
    const [pr, br, er, sr] = await Promise.all([
      authFetch('/api/v1/admin/cardplatform/plans'),
      authFetch('/api/v1/admin/cardplatform/balance'),
      authFetch('/api/v1/admin/network/egress'),
      authFetch('/api/v1/admin/settings'),
    ])
    if (sr.ok) {
      const s = await sr.json()
      configured.value = !!s.card_api_key_configured
    }
    if (er.ok) {
      const e = await er.json()
      egressIp.value = e.egress_ip || ''
    }
    if (pr.ok) {
      const d = await pr.json()
      plans.value = d.plans || {}
      pricingVersion.value = d.version ?? null
      priceSource.value = 'live'
    } else {
      const d = await pr.json().catch(() => ({}))
      metaError.value = d.error || d.msg || '无法获取实时价格（检查 Key / 出口 IP 白名单）'
      plans.value = {
        plus: { label: 'Plus', service_fee_usd: 1, serviceFeeUsdMinor: 100, enabled: true },
        pro_5x: { label: 'Pro 5x', service_fee_usd: 5, serviceFeeUsdMinor: 500, enabled: true },
        pro_20x: { label: 'Pro 20x', service_fee_usd: 10, serviceFeeUsdMinor: 1000, enabled: true },
      }
      priceSource.value = 'docs default'
    }
    if (br.ok) {
      const b = await br.json()
      balanceText.value = String(b.spendable_balance ?? b.balance ?? '')
    }
  } catch (e: any) {
    metaError.value = e?.message || '网络错误'
  } finally {
    loadingMeta.value = false
  }
}

async function issue() {
  issueError.value = ''
  issueOk.value = ''
  if (!canIssue.value) return
  issuing.value = true
  try {
    const r = await authFetch('/api/v1/admin/cardplatform/cdks', {
      method: 'POST',
      body: JSON.stringify({
        plan: form.plan,
        count: form.count,
        funding_confirmed: true,
      }),
    })
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      const msg = d.error || d.msg || '发码失败'
      issueError.value = msg
      if (String(msg).includes('403') || d.code === 403) {
        issueError.value += ' — 可能是 IP 未进白名单，请复制上方出口 IP 到卡台'
      }
      return
    }
    const issued = Array.isArray(d.issued) ? d.issued : (Array.isArray(d.data?.issued) ? d.data.issued : [])
    const codes = issued.map(extractFullCode).filter(Boolean)
    if (!codes.length) {
      issueError.value = '卡台返回成功但未包含完整码字段 code（请勿把列表里的前缀当卡密使用）。原始响应请查网络面板。'
      recentCodes.value = []
      return
    }
    recentCodes.value = codes
    persistRecent(codes, form.plan)
    const shortOnes = codes.filter((c) => !isFullCode(c))
    issueOk.value = shortOnes.length
      ? `成功 ${codes.length} 张，但有 ${shortOnes.length} 张长度异常，请核对`
      : `成功 ${codes.length} 张完整码（每条约 ${codes[0]?.length || '—'} 字符）`
    dialog.toast(issueOk.value, shortOnes.length ? 'warn' : 'ok')
    // 发码成功后自动尝试复制全部，减少漏拷
    await copyAll(false)
    form.funding_confirmed = false
    await loadList()
    await loadMeta()
  } finally {
    issuing.value = false
  }
}

async function loadList() {
  loadingList.value = true
  listError.value = ''
  try {
    const r = await authFetch(`/api/v1/admin/cardplatform/cdks?page=${page.value}&page_size=${pageSize}`)
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      listError.value = d.error || d.msg || '列表失败'
      rows.value = []
      total.value = 0
      return
    }
    rows.value = d.list || []
    total.value = d.total || 0
  } finally {
    loadingList.value = false
  }
}

async function copyAll(showToast = true) {
  if (!recentCodes.value.length) return
  const ok = await copyToClipboard(recentCodes.value.join('\n'))
  if (showToast) dialog.toast(ok ? `已复制 ${recentCodes.value.length} 张完整码` : '复制失败，请用下方文本框全选', ok ? 'ok' : 'err')
}

function downloadCodes() {
  if (!recentCodes.value.length) return
  const blob = new Blob([recentCodes.value.join('\n') + '\n'], { type: 'text/plain;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `cdk-${form.plan || recentMeta.value?.plan || 'batch'}-${Date.now()}.txt`
  a.click()
  URL.revokeObjectURL(a.href)
  dialog.toast('已导出 .txt', 'ok')
}

async function refreshAll() {
  await loadMeta()
  await loadList()
}

onMounted(async () => {
  loadPersistedRecent()
  await refreshAll()
})
</script>

<style scoped>
.plan-card { cursor: pointer; border: 2px solid transparent; }
.plan-card:hover { border-color: var(--brd-2); }
.plan-card--on { border-color: var(--primary) !important; box-shadow: 0 0 0 1px var(--primary-soft); }
.mono { font-variant-numeric: tabular-nums; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.select-all { user-select: all; -webkit-user-select: all; }
</style>
