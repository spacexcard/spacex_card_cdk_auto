<template>
  <div class="space-y-4">

    <!-- 产品状态面板 -->
    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
        <div>
          <h2 class="text-xl font-bold text-ink">产品在线状态</h2>
          <p class="text-sm text-muted mt-1">
            每 3 分钟自动同步一次
            <span v-if="lastSync" class="ml-2 text-subtle">上次：{{ lastSync }}</span>
            <span v-if="nextSync && nextSync !== '—'" class="ml-1 text-subtle">· {{ nextSync }} 后</span>
          </p>
        </div>
        <el-button :loading="syncing" type="primary" plain @click="doSync">立即同步</el-button>
      </div>

      <div v-if="planStatuses.length === 0" class="text-sm text-muted py-4 text-center">
        暂无缓存——点击「立即同步」或等待后台自动同步（需先配置卡台 API Key）
      </div>

      <div v-else class="grid gap-2 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
        <div
          v-for="ps in planStatuses"
          :key="ps.plan_key"
          class="plan-status-card"
          :class="ps.online ? 'status-online' : 'status-offline'"
        >
          <div class="ps-key mono">{{ ps.plan_key }}</div>
          <div class="ps-label">{{ ps.label || ps.plan_key }}</div>
          <div class="ps-fee mono">${{ (ps.service_fee_usd || ps.service_fee_usd_minor / 100 || 0).toFixed(2) }}</div>
          <el-tag :type="ps.online ? 'success' : 'danger'" size="small" effect="dark" class="mt-1">
            {{ ps.online ? '在线' : '已下线' }}
          </el-tag>
        </div>
      </div>
    </div>

    <!-- 选卡优先级规则 -->
    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-2 mb-4">
        <div>
          <h2 class="text-xl font-bold text-ink">自动选卡优先级</h2>
          <p class="text-sm text-muted mt-1">列表顺序 = 优先级，数字越靠前越先用；已下线/禁用的自动跳过</p>
        </div>
        <div class="flex gap-2">
          <el-button @click="addRule">+ 新增</el-button>
          <el-button type="primary" :loading="saving" @click="saveRules">保存</el-button>
        </div>
      </div>

      <div v-if="rules.length === 0" class="text-sm text-muted py-6 text-center">
        暂无规则
      </div>

      <div v-else class="rules-list">
        <div
          v-for="(rule, idx) in rules"
          :key="rule._id"
          class="rule-row"
          :class="{ 'rule-disabled': !rule.enabled }"
        >
          <!-- 序号 -->
          <div class="rule-order">{{ idx + 1 }}</div>

          <!-- 状态徽标 -->
          <div class="rule-status">
            <el-tag
              v-if="planStatusMap[rule.plan_key]"
              :type="planStatusMap[rule.plan_key].online ? 'success' : 'danger'"
              size="small"
              effect="plain"
            >
              {{ planStatusMap[rule.plan_key].online ? '在线' : '已下线' }}
            </el-tag>
            <el-tag v-else type="info" size="small" effect="plain">未同步</el-tag>
          </div>

          <!-- 字段编辑 -->
          <div class="rule-fields">
            <el-input v-model="rule.display_name" placeholder="显示名称" size="small" class="field-name" />
            <el-input v-model="rule.plan_key" placeholder="plan_key（如 537872）" size="small" class="field-key mono" />
            <el-input v-model="rule.bin_prefix" placeholder="BIN前缀" size="small" class="field-bin mono" />
            <el-input v-model="rule.channel" placeholder="渠道（ch1）" size="small" class="field-ch" />
          </div>

          <!-- 启用开关 -->
          <el-switch v-model="rule.enabled" size="small" title="启用此规则" />

          <!-- 上下移 + 删除 -->
          <div class="rule-actions">
            <el-button
              size="small" circle
              :disabled="idx === 0"
              @click="moveUp(idx)"
              title="上移"
            >↑</el-button>
            <el-button
              size="small" circle
              :disabled="idx === rules.length - 1"
              @click="moveDown(idx)"
              title="下移"
            >↓</el-button>
            <el-button size="small" circle type="danger" plain @click="removeRule(idx)" title="删除">×</el-button>
          </div>
        </div>
      </div>

      <div class="mt-3 text-xs text-subtle">
        提示：plan_key 填卡台实际 plan 值（如 <code>537872</code>、<code>525962</code>、<code>usmabo1</code>、<code>plus</code>）；保存后立即生效。
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { authFetch } from '../../lib/api'
import { dialog } from '../../lib/dialog'

interface PlanStatus {
  plan_key: string
  label: string
  online: boolean
  service_fee_usd_minor: number
  service_fee_usd: number
  synced_at: string
}

interface RuleRow {
  _id: number          // 前端临时 id
  id: number
  sort_order: number
  plan_key: string
  display_name: string
  bin_prefix: string
  channel: string
  enabled: boolean
  online: boolean
  synced_at: string
  service_fee_usd: number
}

let _idSeq = 0

const rules = ref<RuleRow[]>([])
const planStatuses = ref<PlanStatus[]>([])
const lastSync = ref('')
const nextSync = ref('')
const saving = ref(false)
const syncing = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

const planStatusMap = computed(() => {
  const m: Record<string, PlanStatus> = {}
  for (const ps of planStatuses.value) m[ps.plan_key] = ps
  return m
})

async function loadRules() {
  const r = await authFetch('/api/v1/admin/card-selection/rules')
  if (!r.ok) return
  const d = await r.json().catch(() => ({}))
  lastSync.value = d.last_sync || ''
  nextSync.value = d.next_sync || ''
  rules.value = (d.rules || []).map((item: any) => ({
    ...item,
    _id: ++_idSeq,
    enabled: item.enabled !== false,
    online: item.online !== false,
  }))
}

async function loadPlanStatus() {
  const r = await authFetch('/api/v1/admin/card-selection/plan-status')
  if (!r.ok) return
  const d = await r.json().catch(() => ({}))
  planStatuses.value = d.statuses || []
  lastSync.value = d.last_sync || lastSync.value
  nextSync.value = d.next_sync || nextSync.value
}

async function doSync() {
  syncing.value = true
  try {
    const r = await authFetch('/api/v1/admin/card-selection/sync', { method: 'POST' })
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      dialog.toast(d.error || '同步失败', 'err')
      return
    }
    planStatuses.value = d.statuses || []
    lastSync.value = d.last_sync || ''
    nextSync.value = d.next_sync || ''
    dialog.toast('同步成功', 'ok')
  } finally {
    syncing.value = false
  }
}

async function saveRules() {
  saving.value = true
  try {
    const payload = rules.value.map((r, i) => ({
      id: r.id || 0,
      sort_order: i + 1,
      plan_key: r.plan_key.trim(),
      display_name: r.display_name.trim() || r.plan_key.trim(),
      bin_prefix: r.bin_prefix.trim(),
      channel: r.channel.trim(),
      enabled: r.enabled,
    }))
    const r = await authFetch('/api/v1/admin/card-selection/rules', {
      method: 'PUT',
      body: JSON.stringify({ rules: payload }),
    })
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      dialog.toast(d.error || '保存失败', 'err')
      return
    }
    // 更新本地 rules（含新 ID）
    rules.value = (d.rules || []).map((item: any) => ({
      ...item,
      _id: ++_idSeq,
      enabled: item.enabled !== false,
      online: item.online !== false,
    }))
    lastSync.value = d.last_sync || lastSync.value
    nextSync.value = d.next_sync || nextSync.value
    dialog.toast('已保存', 'ok')
  } finally {
    saving.value = false
  }
}

function moveUp(idx: number) {
  if (idx === 0) return
  const arr = rules.value
  ;[arr[idx - 1], arr[idx]] = [arr[idx], arr[idx - 1]]
}

function moveDown(idx: number) {
  const arr = rules.value
  if (idx >= arr.length - 1) return
  ;[arr[idx], arr[idx + 1]] = [arr[idx + 1], arr[idx]]
}

function addRule() {
  rules.value.push({
    _id: ++_idSeq,
    id: 0,
    sort_order: rules.value.length + 1,
    plan_key: '',
    display_name: '',
    bin_prefix: '',
    channel: '',
    enabled: true,
    online: true,
    synced_at: '',
    service_fee_usd: 0,
  })
}

function removeRule(idx: number) {
  rules.value.splice(idx, 1)
}

onMounted(async () => {
  await Promise.all([loadRules(), loadPlanStatus()])
  // 每3分钟刷新一次产品状态
  pollTimer = setInterval(loadPlanStatus, 3 * 60 * 1000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<style scoped>
/* 产品状态卡片 */
.plan-status-card {
  padding: 12px 14px;
  border-radius: var(--radius-md);
  border: 1px solid var(--brd);
  background: var(--surface-2);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.status-online { border-left: 3px solid var(--good); }
.status-offline { border-left: 3px solid var(--err); opacity: .7; }
.ps-key { font-size: 13px; font-weight: 700; color: var(--ink); }
.ps-label { font-size: 11px; color: var(--ink-3); }
.ps-fee { font-size: 16px; font-weight: 600; color: var(--primary); }
.mono { font-family: var(--font-mono); }

/* 规则列表 */
.rules-list { display: flex; flex-direction: column; gap: 8px; }

.rule-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: var(--radius-md);
  border: 1px solid var(--brd);
  background: var(--surface);
  transition: .15s ease;
}
.rule-row:hover { border-color: var(--primary); }
.rule-disabled { opacity: .55; }

.rule-order {
  min-width: 22px;
  text-align: center;
  font-size: 13px;
  font-weight: 700;
  color: var(--primary);
}
.rule-status { min-width: 52px; }

.rule-fields {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  flex: 1;
  min-width: 0;
}
.field-name { min-width: 120px; flex: 2; }
.field-key  { min-width: 100px; flex: 1.5; }
.field-bin  { min-width: 80px; flex: 1; }
.field-ch   { min-width: 70px; flex: 1; }

.rule-actions { display: flex; gap: 4px; flex-shrink: 0; }

@media (max-width: 640px) {
  .rule-row { flex-wrap: wrap; }
  .rule-fields { width: 100%; }
}
</style>
