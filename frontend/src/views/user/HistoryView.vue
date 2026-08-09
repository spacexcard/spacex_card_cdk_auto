<template>
  <div class="min-h-screen py-12">
    <div class="max-w-4xl mx-auto px-6">
      <!-- Header -->
      <div class="mb-10 flex items-start justify-between gap-4 animate-slideInUp">
        <div>
          <router-link to="/" class="app-link mb-4 inline-block text-sm">{{ t('common.back') }}</router-link>
          <h1 class="text-3xl font-bold text-ink mb-1">{{ t('history.title') }}</h1>
          <p class="text-muted">{{ t('history.subtitle') }}</p>
        </div>
        <div class="flex items-center gap-3">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </div>

      <!-- Query Section -->
      <div class="card animate-slideInUp space-y-5">
        <h2 class="text-xl font-bold text-ink">{{ t('history.queryTitle') }}</h2>

        <div class="rounded-xl bg-soft p-4 text-sm text-muted">
          {{ t('history.queryHint') }}
        </div>

        <div class="form-group">
          <label>{{ t('history.cdkLabel') }}</label>
          <input
            v-model="cdkCode"
            type="text"
            placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
            class="input mono"
            @keyup.enter="queryTask"
          />
        </div>

        <button
          @click="queryTask"
          :disabled="!cdkCode.trim() || querying"
          class="btn-primary w-full disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <span v-if="!querying">{{ t('history.queryBtn') }}</span>
          <span v-else class="flex items-center justify-center gap-2"><span class="spinner"></span>{{ t('common.querying') }}</span>
        </button>
      </div>

      <!-- Results -->
      <div v-if="taskFound" class="mt-8 card animate-slideInUp">
        <h2 class="text-xl font-bold text-ink mb-6">{{ t('history.resultTitle') }}</h2>

        <div class="space-y-6">
          <div class="rounded-xl bg-soft p-6 space-y-3">
            <div class="flex justify-between items-center">
              <span class="text-muted">{{ t('history.taskId') }}</span>
              <span class="font-mono text-ink">{{ taskInfo.task_id || '—' }}</span>
            </div>
            <div class="flex justify-between items-center">
              <span class="text-muted">{{ t('history.cdkCode') }}</span>
              <span class="font-mono text-ink">{{ taskInfo.cdk_code }}</span>
            </div>
            <div class="flex justify-between items-center">
              <span class="text-muted">{{ t('history.currentStatus') }}</span>
              <span :class="getStatusClass(taskInfo.task_status)">{{ getStatusLabel(taskInfo.task_status) }}</span>
            </div>
            <div class="flex justify-between items-center gap-3">
              <span class="text-muted shrink-0">兑换账号</span>
              <span class="font-mono text-ink text-right break-all">{{ taskInfo.account_email || '—' }}</span>
            </div>
            <div class="flex justify-between items-center">
              <span class="text-muted">银行卡</span>
              <span class="font-mono text-ink">
                <template v-if="taskInfo.card_last_four">•••• {{ taskInfo.card_last_four }}</template>
                <template v-else><span class="text-muted">开卡后显示尾号</span></template>
              </span>
            </div>
            <div v-if="taskInfo.stage" class="flex justify-between items-center text-sm">
              <span class="text-muted">阶段</span>
              <span class="text-muted mono">{{ taskInfo.stage }}</span>
            </div>
            <div v-if="taskInfo.message" class="text-sm text-ink">{{ taskInfo.message }}</div>
            <div v-if="taskInfo.created_at" class="flex justify-between items-center text-sm">
              <span class="text-muted">{{ t('history.submitTime') }}</span>
              <span class="text-muted">{{ formatDate(taskInfo.created_at) }}</span>
            </div>
            <div v-if="taskInfo.completed_at" class="flex justify-between items-center text-sm">
              <span class="text-muted">{{ t('history.completeTime') }}</span>
              <span class="text-muted">{{ taskInfo.completed_at }}</span>
            </div>
          </div>

          <div v-if="taskInfo.task_status === 'pending' || taskInfo.task_status === 'queued'" class="alert" style="background: var(--warn-soft); color: var(--warn); border-color: var(--warn)">{{ t('history.msgPending') }}</div>
          <div v-else-if="taskInfo.task_status === 'submitted' || taskInfo.task_status === 'running'" class="alert alert-info">{{ t('history.msgSubmitted') }}</div>
          <div v-else-if="taskInfo.task_status === 'completed'" class="alert alert-success">{{ t('history.msgCompleted') }}</div>
          <div v-else-if="['failed', 'declined', 'failed_precharge', 'cancelled'].includes(taskInfo.task_status)" class="alert alert-error">{{ t('history.msgFailed') }}</div>
          <div v-else-if="taskInfo.task_status" class="alert alert-info">状态：{{ taskInfo.task_status }}</div>
        </div>

        <button @click="resetQuery" class="btn-secondary w-full mt-6">{{ t('history.queryOther') }}</button>
      </div>

      <!-- Not Found -->
      <div v-if="notFound" class="mt-8 card text-center py-12 animate-slideInUp">
        <p class="text-lg text-ink">{{ notFoundMessage || t('history.notFoundTitle') }}</p>
        <p class="mt-2 text-sm text-muted">{{ t('history.notFoundHint') }}</p>
        <button @click="resetQuery" class="btn-primary mt-6">{{ t('common.retry') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ThemeToggle from '../../components/ThemeToggle.vue'
import LanguageToggle from '../../components/LanguageToggle.vue'

const { t } = useI18n({ useScope: 'global' })

interface Task {
  task_id: string
  cdk_code: string
  account_email?: string
  card_last_four?: string
  task_status: string
  stage?: string
  message?: string
  created_at: string
  updated_at: string
  completed_at?: string
}

function extractCardLastFour(order: any): string {
  const last = String(order?.card_last_four || '').trim()
  if (/^\d{4}$/.test(last)) return last
  const n = String(order?.card_number || '').replace(/\D/g, '')
  return n.length >= 4 ? n.slice(-4) : ''
}

const cdkCode = ref('')
const querying = ref(false)
const taskFound = ref(false)
const notFound = ref(false)
const notFoundMessage = ref('')
const taskInfo = ref<Task>({
  task_id: '',
  cdk_code: '',
  task_status: '',
  created_at: '',
  updated_at: ''
})

function normalizeLookup(data: any, fallbackCode: string): Task {
  // 旧 recharge_tasks 形状
  if (data?.task_id && data?.task_status) {
    return {
      task_id: data.task_id,
      cdk_code: data.cdk_code || fallbackCode,
      account_email: data.account_email,
      card_last_four: data.card_last_four || extractCardLastFour(data),
      task_status: data.task_status,
      stage: data.stage,
      message: data.message,
      created_at: data.created_at || '',
      updated_at: data.updated_at || '',
      completed_at: data.completed_at,
    }
  }
  // 卡台公开 result：{ order: {status,stage,message,account_email,card_last_four,id}, events }
  const order = data?.order || data?.data?.order || data?.data || data || {}
  const st = order.status || data?.status || data?.data?.status || 'unknown'
  const id = order.id || order.order_id || order.client_request_id || data?.task_id || '—'
  return {
    task_id: String(id),
    cdk_code: data?.cdk_code || fallbackCode,
    account_email: String(order.account_email || order.email || data?.account_email || ''),
    card_last_four: extractCardLastFour(order) || String(data?.card_last_four || ''),
    task_status: String(st),
    stage: order.stage || data?.stage || '',
    message: order.message || order.user_message || data?.message || '',
    created_at: order.created_at || data?.created_at || '',
    updated_at: order.updated_at || data?.updated_at || '',
    completed_at: st === 'completed' ? (order.completed_at || order.updated_at) : undefined,
  }
}

const queryTask = async () => {
  if (!cdkCode.value.trim()) return

  querying.value = true
  taskFound.value = false
  notFound.value = false
  notFoundMessage.value = ''
  const code = cdkCode.value.trim()

  try {
    // 优先新接口；失败再走 lookup 兼容
    let response = await fetch(`/api/v1/public/cdk/result-by-code?code=${encodeURIComponent(code)}`)
    let data = await response.json().catch(() => ({}))
    if (!response.ok) {
      response = await fetch(`/api/v1/lookup/task?cdk_code=${encodeURIComponent(code)}`)
      data = await response.json().catch(() => ({}))
    }

    if (response.ok) {
      taskInfo.value = normalizeLookup(data, code)
      taskFound.value = true
    } else {
      notFound.value = true
      notFoundMessage.value = data?.error || data?.message || t('history.notFoundTitle')
    }
  } catch {
    notFound.value = true
    notFoundMessage.value = t('history.notFoundTitle')
  }

  querying.value = false
}

const resetQuery = () => {
  cdkCode.value = ''
  taskFound.value = false
  notFound.value = false
  notFoundMessage.value = ''
}

const getStatusLabel = (status: string) => {
  const key = `history.status.${status}`
  const label = t(key)
  return label === key ? status : label
}

const getStatusClass = (status: string) => {
  const classes: Record<string, string> = {
    pending: 'pill pill-warn',
    submitted: 'pill pill-info',
    completed: 'pill pill-good',
    failed: 'pill pill-err'
  }
  return classes[status] || 'pill'
}

const formatDate = (dateStr: string) => {
  try {
    return new Date(dateStr).toLocaleString()
  } catch {
    return dateStr
  }
}
</script>
