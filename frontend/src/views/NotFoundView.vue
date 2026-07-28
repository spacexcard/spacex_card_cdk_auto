<template>
  <div class="min-h-screen flex items-center justify-center px-6">
    <div class="text-center">
      <h1 class="text-7xl font-bold text-ink mb-3">404</h1>
      <p class="text-lg text-muted mb-8">{{ t('notFound.title') }}</p>
      <router-link to="/" class="btn-primary">{{ t('notFound.home') }}</router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n({ useScope: 'global' })
</script>
