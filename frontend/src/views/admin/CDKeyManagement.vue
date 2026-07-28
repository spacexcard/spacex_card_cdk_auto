<template>
  <div class="pb-2 space-y-5">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <h1 class="text-3xl font-bold text-ink">CDK 卡密</h1>
        <p class="text-sm text-muted mt-2">
          卡台 Open API 发码 · 服务费实时计价 · 完整码仅显示一次
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

        <div v-if="recentCodes.length" class="rounded-xl bg-soft p-4 space-y-2 border" style="border-color: var(--good)">
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium" style="color: var(--good)">完整码（仅此一次）</span>
            <div class="flex gap-1">
              <el-button size="small" type="success" @click="copyAll">全部复制</el-button>
              <el-button size="small" @click="downloadCodes">导出 .txt</el-button>
            </div>
          </div>
          <div
            v-for="c in recentCodes"
            :key="c"
            class="font-mono text-sm break-all cursor-pointer hover:opacity-80"
            style="color: var(--good)"
            title="点击复制"
            @click="copyText(c)"
          >{{ c }}</div>
        </div>
      </section>

      <!-- 列表 -->
      <section class="space-y-3">
        <div class="card flex flex-wrap items-center justify-between gap-3 !py-3">
          <div>
            <h2 class="text-lg font-semibold text-ink">卡台 CDK 列表</h2>
            <p class="text-xs text-muted">仅前缀与状态 · 共 {{ total }} 条</p>
          </div>
          <el-button :loading="loadingList" @click="loadList">刷新</el-button>
        </div>
        <div v-if="listError" class="alert alert-error">{{ listError }}</div>
        <div class="card overflow-hidden !p-0">
          <el-table :data="rows" v-loading="loadingList" size="small" stripe empty-text="暂无数据">
            <el-table-column prop="id" label="ID" width="80" />
            <el-table-column prop="code_prefix" label="前缀" min-width="120">
              <template #default="{ row }">
                <span class="mono">{{ row.code_prefix || '—' }}</span>
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

async function copyText(t: string) {
  try {
    await navigator.clipboard.writeText(t)
    dialog.toast('已复制', 'ok')
  } catch {
    dialog.toast('复制失败', 'err')
  }
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
    const issued = d.issued || []
    recentCodes.value = issued.map((x: any) => x.code).filter(Boolean)
    issueOk.value = `成功 ${issued.length} 张`
    dialog.toast(issueOk.value, 'ok')
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

async function copyAll() {
  await copyText(recentCodes.value.join('\n'))
}
function downloadCodes() {
  const blob = new Blob([recentCodes.value.join('\n') + '\n'], { type: 'text/plain' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `cdk-${form.plan}-${Date.now()}.txt`
  a.click()
  URL.revokeObjectURL(a.href)
}

async function refreshAll() {
  await loadMeta()
  await loadList()
}

onMounted(async () => {
  await refreshAll()
})
</script>

<style scoped>
.plan-card { cursor: pointer; border: 2px solid transparent; }
.plan-card:hover { border-color: var(--brd-2); }
.plan-card--on { border-color: var(--primary) !important; box-shadow: 0 0 0 1px var(--primary-soft); }
.mono { font-variant-numeric: tabular-nums; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
</style>
