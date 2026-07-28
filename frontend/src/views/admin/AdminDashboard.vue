<template>
  <div class="pb-2">
    <div class="w-full space-y-8">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
        <div>
          <h1 class="mt-3 text-3xl font-bold text-ink">管理后台</h1>
          <p class="text-sm text-muted mt-2">
            CDK 白标门户：卡台发码 / 公开兑换 / Webhook / Session 账单查询。
          </p>
        </div>
        <el-button @click="openPwdModal">修改密码</el-button>
      </div>

      <div v-if="statsError" class="alert alert-error">{{ statsError }}</div>
      <div v-if="stats.counts_partial" class="alert alert-warning text-sm">
        CDK 数量较多，状态分布为扫描样本汇总；总数仍以卡台 total 为准。
      </div>

      <div class="grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
        <article v-for="card in statCards" :key="card.label" class="card">
          <div class="text-sm text-muted">{{ card.label }}</div>
          <div class="mt-3 text-3xl font-bold text-ink mono">{{ card.value }}</div>
          <div class="mt-2 text-sm text-subtle">{{ card.hint }}</div>
        </article>
      </div>

      <div class="grid gap-5 md:grid-cols-2">
        <section class="card space-y-3">
          <h2 class="text-xl font-semibold text-ink">快捷入口</h2>
          <div class="flex flex-wrap gap-2">
            <router-link to="/ops/cdkeys" class="btn-primary">CDK 卡密</router-link>
            <router-link to="/ops/orders" class="btn-secondary">兑换对账</router-link>
            <router-link to="/ops/integration" class="btn-secondary">卡台接入</router-link>
            <router-link to="/ops/webhooks" class="btn-secondary">Webhook</router-link>
            <router-link to="/ops/appearance" class="btn-secondary">外观</router-link>
            <router-link to="/billing" class="btn-secondary">Session 账单</router-link>
          </div>
        </section>
        <section class="card space-y-3">
          <h2 class="text-xl font-semibold text-ink">能力说明</h2>
          <ul class="text-sm text-muted list-disc pl-5 space-y-1">
            <li>发码 / 实时服务费：卡台 Open API</li>
            <li>用户兑换：公开 CDK 四步 + 本站 BFF</li>
            <li>对账：开通邮箱 / 用卡 / 金额 / CDK 消耗或释放</li>
            <li>结果通知：轮询 result + 可选 Webhook</li>
          </ul>
        </section>
      </div>
    </div>

    <div
      v-if="pwdModal"
      class="fixed inset-0 z-50 flex items-center justify-center p-4"
      style="background: rgba(0,0,0,0.5)"
      @click.self="closePwdModal"
    >
      <div class="card w-full max-w-md space-y-4">
        <h2 class="text-xl font-semibold text-ink">修改密码</h2>
        <div class="form-group">
          <label>旧密码</label>
          <input v-model="pwdForm.old_password" type="password" class="input" autocomplete="current-password" />
        </div>
        <div class="form-group">
          <label>新密码（至少 12 位）</label>
          <input v-model="pwdForm.new_password" type="password" class="input" autocomplete="new-password" />
        </div>
        <div class="form-group">
          <label>确认新密码</label>
          <input v-model="pwdForm.confirm" type="password" class="input" autocomplete="new-password" />
        </div>
        <div v-if="pwdError" class="alert alert-error">{{ pwdError }}</div>
        <div class="flex gap-2 justify-end">
          <button class="btn-secondary" type="button" @click="closePwdModal">取消</button>
          <button class="btn-primary" type="button" :disabled="pwdLoading" @click="submitPwd">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { authFetch } from '../../lib/api'

const stats = ref<any>({})
const statsError = ref('')
const statsLoading = ref(false)
const pwdModal = ref(false)
const pwdLoading = ref(false)
const pwdError = ref('')
const pwdForm = reactive({ old_password: '', new_password: '', confirm: '' })

function numOrDash(v: unknown) {
  if (v === null || v === undefined || v === '') return '—'
  const n = Number(v)
  return Number.isFinite(n) ? n : '—'
}

const statCards = computed(() => {
  const s = stats.value || {}
  return [
    { label: 'CDK 总数', value: statsLoading.value ? '…' : numOrDash(s.total_cdks ?? s.total_cdkeys), hint: '卡台 Open API' },
    { label: '可用 CDK', value: statsLoading.value ? '…' : numOrDash(s.unused_cdks ?? s.active_cdks ?? s.active_cdkeys), hint: 'unused 未使用' },
    { label: '已兑换', value: statsLoading.value ? '…' : numOrDash(s.consumed_cdks), hint: 'consumed' },
    { label: '兑换订单', value: statsLoading.value ? '…' : numOrDash(s.total_cdk_orders ?? s.total_tasks), hint: '卡台 CDK 订单' },
  ]
})

onMounted(async () => {
  statsLoading.value = true
  statsError.value = ''
  try {
    const r = await authFetch('/api/v1/stats/system')
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      statsError.value = d.error || '加载统计失败'
      return
    }
    stats.value = d
    if (d.error) {
      statsError.value = d.configured === false
        ? '未配置卡台 API Key，请先到「卡台接入」填写'
        : String(d.error)
    }
  } catch (e: any) {
    statsError.value = e?.message || '网络错误'
  } finally {
    statsLoading.value = false
  }
})

function openPwdModal() {
  pwdError.value = ''
  pwdForm.old_password = ''
  pwdForm.new_password = ''
  pwdForm.confirm = ''
  pwdModal.value = true
}
function closePwdModal() {
  pwdModal.value = false
}
async function submitPwd() {
  pwdError.value = ''
  if (pwdForm.new_password !== pwdForm.confirm) {
    pwdError.value = '两次新密码不一致'
    return
  }
  pwdLoading.value = true
  try {
    const r = await authFetch('/api/v1/auth/admin/change-password', {
      method: 'POST',
      body: JSON.stringify({
        old_password: pwdForm.old_password,
        new_password: pwdForm.new_password,
      }),
    })
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      pwdError.value = d.error || '修改失败'
      return
    }
    pwdModal.value = false
  } finally {
    pwdLoading.value = false
  }
}
</script>
