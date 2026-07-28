<template>
  <button
    class="btn-ghost !px-2.5 text-sm font-medium"
    :title="locale === 'zh' ? 'Switch to English' : '切换到中文'"
    :aria-label="locale === 'zh' ? 'Switch to English' : '切换到中文'"
    @click="onToggle"
  >
    {{ locale === 'zh' ? 'EN' : '中文' }}
  </button>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { setLocale, type AppLocale } from '../i18n'

const { locale } = useI18n({ useScope: 'global' })

const onToggle = () => {
  setLocale((locale.value === 'zh' ? 'en' : 'zh') as AppLocale)
}
</script>
