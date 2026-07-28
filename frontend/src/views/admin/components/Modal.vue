<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="$emit('close')">
    <div class="card w-full max-h-[90vh] overflow-y-auto" :class="wide ? 'max-w-3xl' : 'max-w-md'">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold text-ink">{{ title }}</h3>
        <button class="text-muted hover:text-ink text-xl leading-none" @click="$emit('close')">×</button>
      </div>
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ title?: string; wide?: boolean }>()
defineEmits<{ (e: 'close'): void }>()
</script>
