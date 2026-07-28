<template>
  <button
    class="btn-ghost !px-2.5"
    :title="dark ? '切换到亮色' : '切换到暗色'"
    :aria-label="dark ? '切换到亮色' : '切换到暗色'"
    @click="onToggle"
  >
    <!-- sun (shown in dark mode, click → light) -->
    <svg v-if="dark" xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
    </svg>
    <!-- moon (shown in light mode, click → dark) -->
    <svg v-else xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { isDark, toggleTheme, themeMode } from '../theme'

// themeMode is referenced so the computed re-evaluates on change
const dark = computed(() => {
  void themeMode.value
  return isDark()
})

const onToggle = () => toggleTheme()
</script>
