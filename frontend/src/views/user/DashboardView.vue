<template>
  <div class="min-h-screen py-12">
    <div class="max-w-4xl mx-auto px-6">
      <!-- Header -->
      <div class="mb-10 flex items-start justify-between gap-4 animate-slideInUp">
        <div>
          <router-link to="/" class="app-link mb-4 inline-block text-sm">{{ t('common.back') }}</router-link>
          <h1 class="text-3xl font-bold text-ink mb-1">{{ t('dashboard.title') }}</h1>
          <p class="text-muted">{{ t('dashboard.subtitle') }}</p>
        </div>
        <div class="flex items-center gap-3">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </div>

      <!-- Query Section -->
      <div class="card animate-slideInUp space-y-5">
        <div>
          <div class="text-sm font-semibold app-link">STEP 01</div>
          <h3 class="mt-1 text-xl font-bold text-ink">{{ t('dashboard.step1Title') }}</h3>
        </div>

        <div class="rounded-xl bg-soft p-4 text-sm text-muted">
          {{ t('dashboard.hint') }}
        </div>

        <div class="form-group">
          <label>{{ t('dashboard.inputLabel') }}</label>
          <textarea
            v-model="sessionInput"
            :placeholder="t('dashboard.inputPlaceholder')"
            class="input h-32 font-mono"
          ></textarea>
          <p class="text-xs text-subtle">{{ t('dashboard.inputExample') }}</p>
        </div>

        <button
          @click="queryBilling"
          :disabled="!sessionInput.trim() || querying"
          class="btn-primary w-full disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <span v-if="!querying">{{ t('dashboard.queryBtn') }}</span>
          <span v-else class="flex items-center justify-center gap-2"><span class="spinner"></span>{{ t('common.querying') }}</span>
        </button>
      </div>

      <!-- Results -->
      <div v-if="billingFound" class="mt-8 card animate-slideInUp space-y-6">
        <h2 class="text-xl font-bold text-ink">{{ t('dashboard.resultTitle') }}</h2>

        <div class="rounded-xl bg-soft p-6 space-y-3">
          <h3 class="text-base font-semibold text-ink">{{ t('dashboard.accountInfo') }}</h3>
          <div class="flex justify-between items-center">
            <span class="text-muted">{{ t('dashboard.email') }}</span>
            <span class="text-ink">{{ billingInfo.account_email }}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-muted">{{ t('dashboard.subStatus') }}</span>
            <span :class="getStatusClass(billingInfo.subscription_status)">{{ getStatusLabel(billingInfo.subscription_status) }}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-muted">{{ t('dashboard.currentPlan') }}</span>
            <span class="font-semibold text-ink">{{ billingInfo.plan_type }}</span>
          </div>
        </div>

        <div class="rounded-xl bg-soft p-6 space-y-3">
          <h3 class="text-base font-semibold text-ink">{{ t('dashboard.billingInfo') }}</h3>
          <div class="flex justify-between items-center">
            <span class="text-muted">{{ t('dashboard.billingAmount') }}</span>
            <span class="text-2xl font-bold mono" style="color: var(--good)">${{ billingInfo.billing_amount.toFixed(2) }}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-muted">{{ t('dashboard.currency') }}</span>
            <span class="text-ink">{{ billingInfo.currency }}</span>
          </div>
          <div v-if="billingInfo.next_billing_date" class="flex justify-between items-center">
            <span class="text-muted">{{ t('dashboard.nextBilling') }}</span>
            <span class="text-ink">{{ billingInfo.next_billing_date }}</span>
          </div>
        </div>

        <div v-if="billingInfo.subscription_status === 'active'" class="alert alert-success">{{ t('dashboard.msgActive') }}</div>
        <div v-else-if="billingInfo.subscription_status === 'inactive'" class="alert" style="background: var(--warn-soft); color: var(--warn); border-color: var(--warn)">{{ t('dashboard.msgInactive') }}</div>
        <div v-else-if="billingInfo.subscription_status === 'expired'" class="alert alert-error">{{ t('dashboard.msgExpired') }}</div>

        <button @click="resetQuery" class="btn-secondary w-full">{{ t('dashboard.queryOther') }}</button>
      </div>

      <!-- Not Found -->
      <div v-if="notFound" class="mt-8 card text-center py-12 animate-slideInUp">
        <p class="text-lg text-ink">{{ t('dashboard.notFoundTitle') }}</p>
        <p class="mt-2 text-sm text-muted">{{ t('dashboard.notFoundHint') }}</p>
        <button @click="resetQuery" class="btn-primary mt-6">{{ t('common.retry') }}</button>
      </div>

      <!-- Error -->
      <div v-if="queryError" class="mt-8 card text-center py-12 animate-slideInUp">
        <p class="text-lg" style="color: var(--err)">{{ queryError }}</p>
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

interface BillingInfo {
  account_email: string
  subscription_status: string
  plan_type: string
  billing_amount: number
  currency: string
  next_billing_date?: string
}

const sessionInput = ref('')
const querying = ref(false)
const billingFound = ref(false)
const notFound = ref(false)
const queryError = ref('')
const billingInfo = ref<BillingInfo>({
  account_email: '',
  subscription_status: '',
  plan_type: '',
  billing_amount: 0,
  currency: 'USD'
})

const queryBilling = async () => {
  if (!sessionInput.value.trim()) return

  querying.value = true
  billingFound.value = false
  notFound.value = false
  queryError.value = ''

  try {
    const response = await fetch(`/api/v1/billing/query?session=${encodeURIComponent(sessionInput.value)}`)
    const data = await response.json()

    if (response.ok) {
      billingInfo.value = data
      billingFound.value = true
    } else if (response.status === 404) {
      notFound.value = true
    } else {
      queryError.value = data.error || t('dashboard.errQueryFailed')
    }
  } catch (error) {
    queryError.value = t('dashboard.errNetwork')
  }

  querying.value = false
}

const resetQuery = () => {
  sessionInput.value = ''
  billingFound.value = false
  notFound.value = false
  queryError.value = ''
}

const getStatusLabel = (status: string) => {
  const key = `dashboard.status.${status}`
  const label = t(key)
  return label === key ? status : label
}

const getStatusClass = (status: string) => {
  const classes: Record<string, string> = {
    active: 'pill pill-good',
    inactive: 'pill pill-warn',
    expired: 'pill pill-err'
  }
  return classes[status] || 'pill'
}
</script>
