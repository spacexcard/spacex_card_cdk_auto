<template>
  <div class="pb-2">
    <div class="w-full space-y-8">
      <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 class="mt-3 text-3xl font-bold text-ink">操作审计日志</h1>
          <p class="text-sm text-muted mt-2">记录管理员的登录、改密、卡密增删禁用、任务处理、长链接生成等敏感操作。</p>
        </div>
        <div class="flex items-center gap-3">
          <button class="btn-secondary" @click="loadLogs">刷新</button>
        </div>
      </div>

      <div class="card overflow-hidden !p-0">
        <div class="overflow-x-auto">
          <table class="data-table">
            <thead>
              <tr>
                <th>时间</th>
                <th>管理员</th>
                <th>操作</th>
                <th>详情</th>
                <th>IP</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="loading">
                <td colspan="5" class="py-8 text-center text-muted">加载中...</td>
              </tr>
              <tr v-else-if="logs.length === 0">
                <td colspan="5" class="py-8 text-center text-muted">暂无审计记录</td>
              </tr>
              <tr v-for="item in logs" :key="item.id">
                <td class="text-muted whitespace-nowrap">{{ formatDate(item.created_at) }}</td>
                <td class="text-ink">{{ item.username || '-' }}</td>
                <td><span :class="actionClass(item.action)">{{ actionLabel(item.action) }}</span></td>
                <td class="text-muted font-mono break-all">{{ item.detail || '-' }}</td>
                <td class="text-muted font-mono">{{ item.ip || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { authFetch } from '../../lib/api'
import ThemeToggle from '../../components/ThemeToggle.vue'

interface AuditLog {
  id: number
  username: string
  action: string
  detail: string
  ip: string
  created_at: string
}

const logs = ref<AuditLog[]>([])
const loading = ref(false)

const loadLogs = async () => {
  loading.value = true
  try {
    const response = await authFetch('/api/v1/admin/audit-logs?limit=300')
    const data = await response.json()
    logs.value = data.logs || []
  } catch (error) {
    logs.value = []
  }
  loading.value = false
}

const actionLabel = (action: string) => {
  const labels: Record<string, string> = {
    login: '登录',
    change_password: '修改密码',
    cdk_generate: '生成卡密',
    cdk_disable: '禁用卡密',
    cdk_delete: '删除卡密',
    task_approve: '任务完成',
    task_reject: '任务失败',
    checkout_generate: '生成长链接',
  }
  return labels[action] || action
}

const actionClass = (action: string) => {
  const classes: Record<string, string> = {
    login: 'pill pill-info',
    change_password: 'pill pill-warn',
    cdk_generate: 'pill pill-good',
    cdk_disable: 'pill pill-warn',
    cdk_delete: 'pill pill-err',
    task_approve: 'pill pill-good',
    task_reject: 'pill pill-err',
    checkout_generate: 'pill pill-info',
  }
  return classes[action] || 'pill'
}

const formatDate = (value: string) => (value ? new Date(value).toLocaleString('zh-CN') : '-')

onMounted(loadLogs)
</script>
