<template>
  <div class="min-h-screen">
    <!-- Header -->
    <header class="border-b bd">
      <div class="max-w-6xl mx-auto flex items-center justify-between px-6 py-5">
        <div class="flex items-center gap-3">
          <span class="grid h-10 w-10 place-items-center rounded-xl text-white" style="background: var(--primary)">
            <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3h12l4 6-10 12L2 9z" /><path d="M11 3 8 9l4 12 4-12-3-6" /><path d="M2 9h20" /></svg>
          </span>
          <div>
            <h1 class="text-xl font-bold text-ink">{{ brand.name || t('home.brand') }}</h1>
            <p class="text-xs text-muted">{{ brand.sub || t('home.brandSub') }}</p>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <LanguageToggle />
          <!-- 用户仅可切换明暗；整站主题由管理员在 /ops/appearance 设置 -->
          <ThemeToggle />
        </div>
      </div>
    </header>

    <!-- Main Content -->
    <div class="max-w-6xl mx-auto px-6 py-16">
      <!-- Hero -->
      <div class="text-center mb-16 animate-slideInUp">
        <h2 class="text-4xl sm:text-5xl font-bold text-ink mb-4">{{ t('home.heroTitle') }}</h2>
        <p class="text-lg text-muted">{{ t('home.heroSub') }}</p>
      </div>

      <!-- Main Services -->
      <div class="grid md:grid-cols-2 lg:grid-cols-3 gap-6 animate-slideInUp">
        <router-link
          v-for="svc in services"
          :key="svc.to"
          :to="svc.to"
          class="card card-hover group"
        >
          <span class="grid h-12 w-12 place-items-center rounded-2xl" style="background: var(--primary-soft); color: var(--primary)" v-html="svc.icon" />
          <h3 class="mt-4 text-xl font-bold text-ink">{{ svc.title }}</h3>
          <p class="mt-1 text-sm text-subtle">{{ svc.en }}</p>
          <div class="mt-5 space-y-2 text-sm text-muted">
            <div v-for="line in svc.points" :key="line" class="flex items-center gap-2">
              <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4 shrink-0" style="color: var(--primary)" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg>
              <span>{{ line }}</span>
            </div>
          </div>
          <div class="mt-6 text-sm font-medium app-link">{{ svc.cta }} →</div>
        </router-link>
      </div>

      <!-- Flow Section -->
      <div class="mt-20 pt-16 border-t bd">
        <h3 class="text-2xl font-bold text-ink mb-10 text-center">{{ t('home.flowTitle') }}</h3>
        <div class="grid md:grid-cols-2 lg:grid-cols-3 gap-8">
          <div v-for="flow in flows" :key="flow.title" class="space-y-4">
            <h4 class="font-bold text-ink">{{ flow.title }}</h4>
            <div class="space-y-3 text-sm text-muted">
              <div v-for="(step, i) in flow.steps" :key="i" class="flex gap-3">
                <span class="grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-bold" style="background: var(--primary-soft); color: var(--primary)">{{ i + 1 }}</span>
                <span>{{ step }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ThemeToggle from '../components/ThemeToggle.vue'
import LanguageToggle from '../components/LanguageToggle.vue'
import { siteBrand } from '../theme'

const brand = siteBrand

const { t } = useI18n({ useScope: 'global' })

const services = computed(() => [
  {
    to: '/recharge',
    title: t('home.services.recharge.title'),
    en: t('home.services.recharge.en'),
    cta: t('home.services.recharge.cta'),
    icon: '<svg xmlns="http://www.w3.org/2000/svg" class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M9 14l2 2 4-4"/></svg>',
    points: [t('home.services.recharge.p1'), t('home.services.recharge.p2'), t('home.services.recharge.p3')],
  },
  {
    to: '/batch',
    title: t('home.services.batch.title'),
    en: t('home.services.batch.en'),
    cta: t('home.services.batch.cta'),
    icon: '<svg xmlns="http://www.w3.org/2000/svg" class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>',
    points: [t('home.services.batch.p1'), t('home.services.batch.p2'), t('home.services.batch.p3')],
  },

  {
    to: '/billing',
    title: t('home.services.billing.title'),
    en: t('home.services.billing.en'),
    cta: t('home.services.billing.cta'),
    icon: '<svg xmlns="http://www.w3.org/2000/svg" class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>',
    points: [t('home.services.billing.p1'), t('home.services.billing.p2'), t('home.services.billing.p3')],
  },
])

const flows = computed(() => [
  { title: t('home.flows.submit.title'), steps: [t('home.flows.submit.s1'), t('home.flows.submit.s2'), t('home.flows.submit.s3')] },
  { title: t('home.flows.batch.title'), steps: [t('home.flows.batch.s1'), t('home.flows.batch.s2'), t('home.flows.batch.s3')] },
  { title: t('home.flows.billing.title'), steps: [t('home.flows.billing.s1'), t('home.flows.billing.s2'), t('home.flows.billing.s3')] },
])
</script>
