<template>
  <div class="space-y-4">
    <el-card shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">卡台 Webhook 回调</span>
          <el-tag size="small" type="success">验签 + 幂等入库</el-tag>
        </div>
      </template>
      <p class="text-sm text-muted mb-3">
        兑换结果仍可用轮询 <code>/public/cdk/result</code>；配置 Webhook 后卡台会主动推送
        <code>gpt_direct.completed</code> / 卡交易等事件（OpenAPI §7）。
      </p>
      <el-form label-width="140px" class="max-w-2xl">
        <el-form-item label="回调 URL">
          <el-input :model-value="webhookUrl" readonly>
            <template #append>
              <el-button @click="copy(webhookUrl)">复制</el-button>
            </template>
          </el-input>
          <div class="text-xs text-subtle mt-1">填到卡台「开发者」页的回调地址（须 https）</div>
        </el-form-item>
        <el-form-item label="Webhook Secret">
          <el-input
            v-model="secretInput"
            type="password"
            show-password
            :placeholder="secretHint"
          />
          <div class="text-xs text-subtle mt-1">
            卡台首次保存回调地址后生成的 <code>whsec_…</code>；本站用其校验 <code>X-Signature</code>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveSecret">保存 Secret</el-button>
          <el-button :loading="loading" @click="load">刷新事件</el-button>
        </el-form-item>
      </el-form>
      <el-alert
        v-if="!secretSet"
        type="warning"
        :closable="false"
        show-icon
        title="尚未配置 webhook_secret：卡台回调会被 503/401 拒绝"
        class="mt-2"
      />
    </el-card>

    <el-card shadow="never" header="最近事件">
      <div v-if="error" class="alert alert-error mb-3">{{ error }}</div>
      <table class="data-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>类型</th>
            <th>幂等键</th>
            <th>时间</th>
            <th>摘要</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="5" class="py-6 text-center text-muted">加载中…</td>
          </tr>
          <tr v-else-if="!events.length">
            <td colspan="5" class="py-6 text-center text-muted">暂无回调（先配 URL + secret，或继续用轮询）</td>
          </tr>
          <tr v-for="e in events" :key="e.id">
            <td class="mono">{{ e.id }}</td>
            <td>{{ e.event_type }}</td>
            <td class="mono text-xs">{{ e.idem_key }}</td>
            <td class="text-sm text-muted">{{ e.created_at }}</td>
            <td class="text-xs text-subtle">{{ summarize(e.payload) }}</td>
          </tr>
        </tbody>
      </table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { authFetch } from '../../lib/api'
import { dialog } from '../../lib/dialog'

const webhookUrl = ref('https://gptcdk.ai/api/v1/webhooks/cardplatform')
const secretSet = ref(false)
const secretHint = ref('未配置')
const secretInput = ref('')
const events = ref<any[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')

function summarize(p: any) {
  if (!p || typeof p !== 'object') return '—'
  if (p.type === 'gpt_direct.completed' || p.order_id) {
    return `order=${p.order_id || ''} plan=${p.plan || ''} status=${p.status || ''}`
  }
  if (p.event === 'card_transaction') {
    return `${p.type || ''} ${p.status || ''} ${p.merchant_name || p.merchant || ''}`
  }
  if (p.event === 'card_operation') {
    return `${p.operation || ''} ${p.status || ''}`
  }
  return Object.keys(p).slice(0, 4).join(',')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const r = await authFetch('/api/v1/admin/webhooks/events')
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      error.value = d.error || '加载失败'
      return
    }
    if (d.webhook_url) webhookUrl.value = d.webhook_url
    secretSet.value = !!d.webhook_secret_set
    secretHint.value = d.webhook_secret_set ? `已配置 ${d.webhook_secret_hint || ''}` : '未配置'
    events.value = d.events || []
  } finally {
    loading.value = false
  }
}

async function saveSecret() {
  if (!secretInput.value.trim()) {
    dialog.toast('请填写 secret', 'err')
    return
  }
  saving.value = true
  try {
    const r = await authFetch('/api/v1/admin/settings', {
      method: 'PUT',
      body: JSON.stringify({ webhook_secret: secretInput.value.trim() }),
    })
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      dialog.toast(d.error || '保存失败', 'err')
      return
    }
    secretInput.value = ''
    dialog.toast('已保存 webhook_secret', 'ok')
    await load()
  } finally {
    saving.value = false
  }
}

async function copy(t: string) {
  try {
    await navigator.clipboard.writeText(t)
    dialog.toast('已复制', 'ok')
  } catch {
    dialog.toast('复制失败', 'err')
  }
}

onMounted(load)
</script>
