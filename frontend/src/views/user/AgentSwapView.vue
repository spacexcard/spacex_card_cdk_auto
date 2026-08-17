<template>
  <div class="min-h-screen bg-canvas px-4 py-10">
    <div class="mx-auto w-full max-w-md space-y-4">
      <div class="card space-y-1">
        <h1 class="text-xl font-bold text-ink">卡密补发</h1>
        <p class="text-sm text-muted">
          仅限<strong>充值失败且未扣款</strong>的卡密。验证通过后将作废旧码并下发一张同套餐新码。
        </p>
      </div>

      <div class="card space-y-4">
        <div>
          <label class="text-xs text-muted">代理密码</label>
          <el-input
            v-model="password"
            type="password"
            show-password
            size="large"
            placeholder="管理员下发的个人密码"
            autocomplete="off"
          />
        </div>
        <div>
          <label class="text-xs text-muted">失败的完整卡密</label>
          <el-input
            v-model="code"
            type="textarea"
            :rows="3"
            size="large"
            placeholder="粘贴完整 CDK"
            class="mono"
          />
        </div>
        <el-button
          type="primary"
          size="large"
          class="!w-full"
          :loading="busy"
          :disabled="!password.trim() || !code.trim()"
          @click="doExchange"
        >
          {{ busy ? '处理中…' : '换发新卡密' }}
        </el-button>
        <p v-if="error" class="alert alert-error text-sm">{{ error }}</p>
      </div>

      <div v-if="result" class="card space-y-3">
        <div class="alert alert-success text-sm">{{ result.message || '换发成功' }}</div>
        <div>
          <div class="text-xs text-muted mb-1">新卡密（仅显示一次，请立即复制）</div>
          <div class="mono break-all text-sm p-3 rounded-lg bg-muted/20 border border-line">
            {{ result.new_code }}
          </div>
        </div>
        <div class="flex gap-2">
          <el-button type="primary" @click="copyNew">复制新码</el-button>
          <el-button @click="reset">再换一张</el-button>
        </div>
        <div class="text-xs text-subtle space-y-0.5">
          <div>套餐：{{ result.plan || '—' }}</div>
          <div>旧单状态：{{ result.order_status || '—' }}</div>
          <div>旧码前缀：{{ result.old_code_prefix || '—' }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { dialog } from '../../lib/dialog'
import { copyToClipboard } from '../../lib/clipboard'

const password = ref('')
const code = ref('')
const busy = ref(false)
const error = ref('')
const result = ref<any>(null)

async function doExchange() {
  error.value = ''
  result.value = null
  busy.value = true
  try {
    const r = await fetch('/api/v1/public/cdk/exchange', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        password: password.value.trim(),
        code: code.value.trim(),
      }),
    })
    const d = await r.json().catch(() => ({}))
    if (!r.ok) {
      error.value = d.error || d.msg || `失败 (${r.status})`
      return
    }
    result.value = d
    dialog.toast('已换发新码，请立即复制', 'ok')
  } catch (e: any) {
    error.value = e?.message || '网络错误'
  } finally {
    busy.value = false
  }
}

async function copyNew() {
  const c = String(result.value?.new_code || '')
  if (!c) return
  const ok = await copyToClipboard(c)
  dialog.toast(ok ? '已复制' : '复制失败，请手动全选', ok ? 'ok' : 'err')
}

function reset() {
  code.value = ''
  result.value = null
  error.value = ''
}
</script>
