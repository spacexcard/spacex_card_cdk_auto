<template>
  <div class="min-h-screen py-12">
    <div class="max-w-4xl mx-auto px-6">
      <div class="mb-10 flex items-start justify-between gap-4 animate-slideInUp">
        <div>
          <router-link to="/" class="app-link mb-4 inline-block text-sm">{{ t('common.back') }}</router-link>
          <h1 class="text-3xl font-bold text-ink mb-1">{{ t('cdkLookup.title') }}</h1>
          <p class="text-muted">{{ t('cdkLookup.subtitle') }}</p>
        </div>
        <div class="flex items-center gap-3">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </div>

      <RedeemModeTabs />

      <div class="card animate-slideInUp space-y-5">
        <h2 class="text-xl font-bold text-ink">{{ t('cdkLookup.queryTitle') }}</h2>
        <div class="rounded-xl bg-soft p-4 text-sm text-muted">
          {{ t('cdkLookup.queryHint') }}
        </div>
        <div class="form-group">
          <label>{{ t('cdkLookup.cdkLabel') }}</label>
          <input
            v-model="cdkCode"
            type="text"
            :placeholder="t('cdkLookup.cdkPlaceholder')"
            class="input mono"
            @keyup.enter="query"
          />
        </div>
        <button
          @click="query"
          :disabled="!cdkCode.trim() || querying"
          class="btn-primary w-full disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <span v-if="!querying">{{ t('cdkLookup.queryBtn') }}</span>
          <span v-else class="flex items-center justify-center gap-2">
            <span class="spinner"></span>{{ t('common.querying') }}
          </span>
        </button>
        <p v-if="error" class="text-sm" style="color: var(--err)">{{ error }}</p>
      </div>

      <div v-if="result" class="mt-8 card animate-slideInUp">
        <h2 class="text-xl font-bold text-ink mb-6">{{ t('cdkLookup.resultTitle') }}</h2>
        <div class="rounded-xl bg-soft p-6 space-y-4">
          <div class="flex justify-between items-center gap-3">
            <span class="text-muted shrink-0">{{ t('cdkLookup.cdkCode') }}</span>
            <span class="font-mono text-ink text-right break-all">{{ result.cdk_code }}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-muted">{{ t('cdkLookup.useStatus') }}</span>
            <span :class="statusClass">{{ statusLabel }}</span>
          </div>
          <div class="flex justify-between items-center gap-3">
            <span class="text-muted shrink-0">{{ t('cdkLookup.rechargeEmail') }}</span>
            <span class="font-mono text-ink text-right break-all">
              {{ result.account_email || t('cdkLookup.emailEmpty') }}
            </span>
          </div>
          <div v-if="result.plan" class="flex justify-between items-center">
            <span class="text-muted">{{ t('cdkLookup.plan') }}</span>
            <span class="text-ink">{{ result.plan }}</span>
          </div>
          <div v-if="result.used_at" class="flex justify-between items-center text-sm">
            <span class="text-muted">{{ t('cdkLookup.usedAt') }}</span>
            <span class="text-muted">{{ result.used_at }}</span>
          </div>
        </div>

        <div
          v-if="result.used"
          class="alert alert-success mt-6"
        >{{ result.message || t('cdkLookup.msgUsed') }}</div>
        <div
          v-else-if="result.status === 'processing'"
          class="alert alert-info mt-6"
        >{{ result.message || t('cdkLookup.msgProcessing') }}</div>
        <div
          v-else-if="result.status === 'disabled' || result.status === 'expired'"
          class="alert mt-6"
          style="background: var(--warn-soft); color: var(--warn); border-color: var(--warn)"
        >{{ result.message }}</div>
        <div
          v-else
          class="alert alert-info mt-6"
        >{{ result.message || t('cdkLookup.msgUnused') }}</div>

        <button @click="reset" class="btn-secondary w-full mt-6">{{ t('cdkLookup.queryOther') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ThemeToggle from '../../components/ThemeToggle.vue'
import LanguageToggle from '../../components/LanguageToggle.vue'
import RedeemModeTabs from '../../components/RedeemModeTabs.vue'

const { t } = useI18n({ useScope: 'global' })

interface CDKStatusResult {
  cdk_code: string
  status: string
  used: boolean
  account_email?: string
  plan?: string
  used_at?: string
  message?: string
}

const cdkCode = ref('')
const querying = ref(false)
const error = ref('')
const result = ref<CDKStatusResult | null>(null)

const statusLabel = computed(() => {
  const s = result.value?.status || ''
  const key = `cdkLookup.status.${s}`
  const label = t(key)
  return label === key ? s : label
})

const statusClass = computed(() => {
  const s = result.value?.status
  if (s === 'used') return 'font-semibold'
  if (s === 'unused') return 'font-semibold'
  if (s === 'processing') return 'font-semibold'
  return 'font-semibold'
})

async function query() {
  const code = cdkCode.value.trim()
  if (!code) return
  querying.value = true
  error.value = ''
  result.value = null
  try {
    const r = await fetch(`/api/v1/lookup/cdk?code=${encodeURIComponent(code)}`)
    const data = await r.json().catch(() => ({}))
    if (!r.ok) {
      error.value = data.error || data.message || t('cdkLookup.errNotFound')
      return
    }
    result.value = data as CDKStatusResult
  } catch {
    error.value = t('cdkLookup.errNetwork')
  } finally {
    querying.value = false
  }
}

function reset() {
  result.value = null
  error.value = ''
  cdkCode.value = ''
}
</script>
