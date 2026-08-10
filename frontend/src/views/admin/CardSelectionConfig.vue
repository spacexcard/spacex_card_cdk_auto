<template>
  <div class="space-y-4">

    <!-- 产品在线状态 -->
    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
        <div>
          <h2 class="text-xl font-bold text-ink">产品在线状态</h2>
          <p class="text-sm text-muted mt-1">
            每 3 分钟自动同步 · 共 <strong>{{ products.length }}</strong> 个产品
            <span v-if="lastSync" class="ml-2 text-subtle">上次：{{ lastSync }}</span>
            <span v-if="nextSync && nextSync !== '—'" class="ml-1 text-subtle">· {{ nextSync }}后</span>
          </p>
        </div>
        <el-button :loading="syncing" type="primary" plain @click="doSync">立即同步</el-button>
      </div>

      <div v-if="products.length === 0" class="text-sm text-muted py-6 text-center">
        暂无产品缓存——点击「立即同步」（需先在「卡台接入」配置 API Key）
      </div>

      <div v-else class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        <div
          v-for="p in products"
          :key="p.product_code"
          class="prod-card"
          :class="isProductOnline(p) ? 'prod-online' : 'prod-offline'"
        >
          <div class="flex items-start justify-between gap-1">
            <span class="prod-code mono">{{ p.product_code }}</span>
            <el-tag :type="isProductOnline(p) ? 'success' : 'danger'" size="small" effect="dark">
              {{ isProductOnline(p) ? '在线' : '已下线' }}
            </el-tag>
          </div>
          <div class="prod-issuer">{{ issuerLabel(p.issuer) }}</div>
          <div class="prod-bin mono">{{ binDisplay(p) }}</div>
          <div class="prod-area">{{ p.issuing_area }} · {{ p.scene }}</div>
          <div v-if="p.suspended_at" class="prod-suspend">已暂停</div>
        </div>
      </div>
    </div>

    <!-- 自动选卡优先级 -->
    <div class="card">
      <div class="flex flex-wrap items-center justify-between gap-2 mb-3">
        <div>
          <h2 class="text-xl font-bold text-ink">自动选卡优先级</h2>
          <p class="text-sm text-muted mt-1">
            顺序越靠前优先级越高；已下线或禁用的自动跳过
            <el-tag type="warning" size="small" effect="plain" class="ml-2">仅美卡参与自动选卡</el-tag>
          </p>
        </div>
        <div class="flex gap-2">
          <el-button plain @click="showAddDialog = true">+ 添加产品</el-button>
          <el-button type="primary" :loading="saving" @click="saveRules">保存</el-button>
        </div>
      </div>

      <div v-if="rules.length === 0" class="text-sm text-muted py-8 text-center">暂无规则</div>

      <div v-else class="rules-list">
        <div
          v-for="(rule, idx) in rules"
          :key="rule._id"
          class="rule-row"
          :class="{ 'rule-disabled': !rule.enabled }"
        >
          <!-- 优先级序号 -->
          <div class="rule-num">{{ idx + 1 }}</div>

          <!-- 状态 -->
          <div class="rule-badge">
            <el-tag
              v-if="productMap[rule.plan_key]"
              :type="isProductOnline(productMap[rule.plan_key]) ? 'success' : 'danger'"
              size="small" effect="plain"
            >{{ isProductOnline(productMap[rule.plan_key]) ? '在线' : '已下线' }}</el-tag>
            <el-tag v-else type="info" size="small" effect="plain">未同步</el-tag>
          </div>

          <!-- 产品信息（只读展示 + 渠道可编辑） -->
          <div class="rule-info">
            <div class="rule-name">
              <span class="issuer-tag" :data-issuer="productMap[rule.plan_key]?.issuer || ''">
                {{ productMap[rule.plan_key] ? issuerLabel(productMap[rule.plan_key].issuer) : rule.channel || '—' }}
              </span>
              <span class="rule-code mono">{{ rule.plan_key }}</span>
              <span v-if="productMap[rule.plan_key]" class="rule-scene text-muted">
                · {{ productMap[rule.plan_key].scene }}
              </span>
            </div>
            <div class="rule-bins mono text-subtle">
              {{ productMap[rule.plan_key] ? binDisplay(productMap[rule.plan_key]) : (rule.bin_prefix || '—') }}
            </div>
          </div>

          <!-- 启用开关 -->
          <el-switch v-model="rule.enabled" size="small" />

          <!-- 上下移 + 删除 -->
          <div class="rule-actions">
            <el-button size="small" circle :disabled="idx === 0" @click="moveUp(idx)" title="上移">↑</el-button>
            <el-button size="small" circle :disabled="idx === rules.length - 1" @click="moveDown(idx)" title="下移">↓</el-button>
            <el-button size="small" circle type="danger" plain @click="removeRule(idx)" title="移除">×</el-button>
          </div>
        </div>
      </div>

      <div class="mt-3 text-xs text-subtle">
        ※ 仅美卡（渠道1/渠道3/渠道4）参与默认优先级；香港卡可手动添加，但默认排除。
      </div>
    </div>

    <!-- 添加产品弹窗 -->
    <el-dialog v-model="showAddDialog" title="添加产品到优先级" width="480px" align-center destroy-on-close>
      <div class="space-y-3">
        <p class="text-sm text-muted">选择要加入自动选卡的产品（已在列表中的不会重复添加）</p>
        <div
          v-for="p in addableProducts"
          :key="p.product_code"
          class="add-prod-row"
          :class="{ 'add-prod-offline': !isProductOnline(p) }"
          @click="isProductOnline(p) && addProductToRules(p)"
        >
          <div class="flex items-center gap-2 flex-1">
            <span class="issuer-tag" :data-issuer="p.issuer">{{ issuerLabel(p.issuer) }}</span>
            <span class="mono font-semibold">{{ p.product_code }}</span>
            <span class="text-sm text-muted">{{ p.scene }}</span>
          </div>
          <div class="text-xs text-subtle mono">{{ binDisplay(p) }}</div>
          <el-tag v-if="!isProductOnline(p)" type="danger" size="small">已下线</el-tag>
          <el-tag v-else-if="isInRules(p.product_code)" type="info" size="small">已添加</el-tag>
          <el-tag v-else type="success" size="small">+ 添加</el-tag>
        </div>
        <p v-if="addableProducts.length === 0" class="text-sm text-muted text-center py-4">
          暂无可添加的产品，请先点击「立即同步」
        </p>
      </div>
      <template #footer>
        <el-button @click="showAddDialog = false">关闭</el-button>
      </template>
    </el-dialog>

  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { authFetch } from '../../lib/api'
import { dialog } from '../../lib/dialog'

interface CardProduct {
  product_code: string
  issuer: string
  bin: string
  network: string
  issuing_area: string
  scene: string
  card_group: string
  description: string
  bin_heads: string[]
  enabled: boolean
  suspended_at: string
  synced_at: string
}

interface RuleRow {
  _id: number
  id: number
  sort_order: number
  plan_key: string
  display_name: string
  bin_prefix: string
  channel: string
  enabled: boolean
}

let _idSeq = 0
const rules = ref<RuleRow[]>([])
const products = ref<CardProduct[]>([])
const lastSync = ref('')
const nextSync = ref('')
const saving = ref(false)
const syncing = ref(false)
const showAddDialog = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

const ISSUER_MAP: Record<string, string> = {
  one: '渠道1', two: '渠道2', three: '渠道3', four: '渠道4', five: '渠道5',
}

function issuerLabel(issuer: string) {
  return ISSUER_MAP[issuer] || issuer || '—'
}

function isProductOnline(p: CardProduct) {
  return p.enabled && !p.suspended_at
}

function binDisplay(p: CardProduct) {
  // bin 字段是完整卡号前缀（可能8位，如 43612080）
  // bin_heads 是 6 位 BIN 列表（多卡段时用）
  // 规则：
  //  - 若 bin 比 bin_heads 中任何一个都长，说明 bin 是完整前缀，优先展示 bin
  //  - 若存在多个 bin_heads 且 bin 已包含在其中，展示所有 bin_heads
  //  - 否则 fallback 到 bin
  const bin = p.bin || ''
  const heads = (p.bin_heads || []).filter(Boolean)

  if (heads.length === 0) return bin || '—'

  // bin 比所有 bin_heads 都长（8位 bin vs 6位 heads）→ 优先用完整 bin 展示
  const maxHeadLen = Math.max(...heads.map(h => h.length))
  if (bin.length > maxHeadLen) {
    // 若同时有多个 bin_heads（多卡段产品），展示完整 bin + 其他 bin_heads
    if (heads.length > 1) {
      // 找到与 bin 对应的那个 head（前缀匹配）
      const others = heads.filter(h => !bin.startsWith(h))
      return others.length ? `${bin} / ${others.join(' / ')}` : bin
    }
    return bin
  }

  // bin_heads 已经够详细（含多卡段），直接展示
  return heads.join(' / ')
}

// product_code → product map
const productMap = computed(() => {
  const m: Record<string, CardProduct> = {}
  for (const p of products.value) m[p.product_code] = p
  return m
})

// 产品列表（全量，用于弹窗选择）
const addableProducts = computed(() => products.value)

function isInRules(code: string) {
  return rules.value.some(r => r.plan_key === code)
}

function addProductToRules(p: CardProduct) {
  if (isInRules(p.product_code)) {
    dialog.toast('该产品已在列表中', 'warn')
    return
  }
  rules.value.push({
    _id: ++_idSeq,
    id: 0,
    sort_order: rules.value.length + 1,
    plan_key: p.product_code,
    display_name: `${issuerLabel(p.issuer)} · ${p.product_code} · ${p.scene}`,
    bin_prefix: p.bin_heads?.[0] || p.bin || '',
    channel: p.issuer,
    enabled: true,
  })
  dialog.toast(`已添加 ${p.product_code}`, 'ok')
}

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
  }))
}

async function loadPlanStatus() {
  const r = await authFetch('/api/v1/admin/card-selection/plan-status')
  if (!r.ok) return
  const d = await r.json().catch(() => ({}))
  products.value = d.products || []
  lastSync.value = d.last_sync || lastSync.value
  nextSync.value = d.next_sync || nextSync.value
}

async function doSync() {
  syncing.value = true
  try {
    const r = await authFetch('/api/v1/admin/card-selection/sync', { method: 'POST' })
    const d = await r.json().catch(() => ({}))
    if (!r.ok) { dialog.toast(d.error || '同步失败', 'err'); return }
    products.value = d.products || []
    lastSync.value = d.last_sync || ''
    nextSync.value = d.next_sync || ''
    dialog.toast(`同步完成，共 ${products.value.length} 个产品`, 'ok')
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
      bin_prefix: r.bin_prefix?.trim() || '',
      channel: r.channel?.trim() || '',
      enabled: r.enabled,
    }))
    const r = await authFetch('/api/v1/admin/card-selection/rules', {
      method: 'PUT',
      body: JSON.stringify({ rules: payload }),
    })
    const d = await r.json().catch(() => ({}))
    if (!r.ok) { dialog.toast(d.error || '保存失败', 'err'); return }
    rules.value = (d.rules || []).map((item: any) => ({
      ...item, _id: ++_idSeq, enabled: item.enabled !== false,
    }))
    dialog.toast('已保存', 'ok')
  } finally {
    saving.value = false
  }
}

function moveUp(idx: number) {
  if (idx === 0) return
  const a = rules.value
  ;[a[idx - 1], a[idx]] = [a[idx], a[idx - 1]]
}
function moveDown(idx: number) {
  const a = rules.value
  if (idx >= a.length - 1) return
  ;[a[idx], a[idx + 1]] = [a[idx + 1], a[idx]]
}
function removeRule(idx: number) {
  rules.value.splice(idx, 1)
}

onMounted(async () => {
  await Promise.all([loadRules(), loadPlanStatus()])
  pollTimer = setInterval(loadPlanStatus, 3 * 60 * 1000)
})
onUnmounted(() => { if (pollTimer) clearInterval(pollTimer) })
</script>

<style scoped>
/* ── 产品状态卡片 ── */
.prod-card {
  padding: 11px 13px;
  border-radius: var(--radius-md);
  border: 1px solid var(--brd);
  background: var(--surface-2);
  display: flex; flex-direction: column; gap: 3px;
}
.prod-online  { border-left: 3px solid var(--good); }
.prod-offline { border-left: 3px solid var(--err); opacity: .6; }
.prod-code   { font-size: 14px; font-weight: 700; color: var(--ink); }
.prod-issuer { font-size: 11px; font-weight: 600; color: var(--primary); }
.prod-bin    { font-size: 12px; color: var(--ink-2); }
.prod-area   { font-size: 11px; color: var(--ink-3); }
.prod-suspend{ font-size: 11px; color: var(--err); }
.mono { font-family: var(--font-mono); }

/* ── 渠道标签 ── */
.issuer-tag {
  display: inline-block;
  padding: 1px 7px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  background: var(--primary-soft);
  color: var(--primary);
  white-space: nowrap;
}
[data-issuer="one"]   { background: #eff6ff; color: #2563eb; }
[data-issuer="three"] { background: #f0fdf4; color: #16a34a; }
[data-issuer="four"]  { background: #fef9c3; color: #854d0e; }

/* ── 规则列表 ── */
.rules-list { display: flex; flex-direction: column; gap: 6px; }

.rule-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: var(--radius-md);
  border: 1px solid var(--brd);
  background: var(--surface);
  transition: .15s ease;
}
.rule-row:hover { border-color: var(--primary); box-shadow: var(--shadow-sm); }
.rule-disabled  { opacity: .45; }

.rule-num {
  min-width: 24px; text-align: center;
  font-size: 14px; font-weight: 800; color: var(--primary); flex-shrink: 0;
}
.rule-badge { min-width: 56px; flex-shrink: 0; }

.rule-info { flex: 1; min-width: 0; }
.rule-name {
  display: flex; align-items: center; flex-wrap: wrap; gap: 6px;
  font-size: 14px;
}
.rule-code  { font-weight: 700; color: var(--ink); }
.rule-scene { font-size: 12px; }
.rule-bins  { font-size: 11px; margin-top: 3px; }

.rule-actions { display: flex; gap: 4px; flex-shrink: 0; }

/* ── 添加产品弹窗行 ── */
.add-prod-row {
  display: flex; align-items: center; gap: 10px;
  padding: 10px 12px; border-radius: var(--radius-md);
  border: 1px solid var(--brd); background: var(--surface-2);
  cursor: pointer; transition: .15s;
}
.add-prod-row:hover { border-color: var(--primary); background: var(--primary-soft); }
.add-prod-offline   { opacity: .5; cursor: not-allowed; }

@media (max-width: 640px) {
  .rule-row { flex-wrap: wrap; }
  .rule-info { width: 100%; }
}
</style>
