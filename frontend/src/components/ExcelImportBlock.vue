<template>
  <div class="space-y-1.5">
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <label class="block text-sm font-medium text-ink">{{ t('batch.excelLabel') }}</label>
      <button
        v-if="sessionPool.length > 0"
        type="button"
        class="text-xs text-muted hover:underline"
        style="color: var(--err, #dc2626)"
        @click="$emit('clear')"
      >
        {{ t('batch.clearImport') }}
      </button>
    </div>
    <input
      ref="fileInputRef"
      type="file"
      accept=".xlsx,.xls,.csv"
      class="hidden"
      @change="onChange"
    />
    <button
      type="button"
      class="w-full py-2.5 rounded-xl border border-dashed text-sm font-medium disabled:opacity-40"
      style="border-color: color-mix(in srgb, var(--primary) 45%, var(--brd)); color: var(--primary)"
      :disabled="importing"
      @click="fileInputRef?.click()"
    >
      {{
        importing
          ? t('batch.importing')
          : sessionPool.length
            ? t('batch.imported', { n: sessionPool.length })
            : t('batch.importBtn')
      }}
    </button>
    <p v-if="importMsg" class="text-xs text-muted leading-relaxed">{{ importMsg }}</p>
    <div
      v-if="sessionPool.length > 0"
      class="rounded-lg bg-soft px-3 py-2 text-[11px] text-muted max-h-24 overflow-y-auto font-mono"
    >
      <div v-for="(s, i) in sessionPool.slice(0, 8)" :key="i">
        {{ i + 1 }}. {{ s.email || '(无邮箱)' }} · session {{ s.session.length }} 字
      </div>
      <div v-if="sessionPool.length > 8">…共 {{ sessionPool.length }} 条</div>
    </div>
    <p class="text-[11px] text-subtle">{{ t('batch.excelHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ImportedSession } from '../lib/batch-session'

defineProps<{
  sessionPool: ImportedSession[]
  importMsg: string
  importing: boolean
}>()

const emit = defineEmits<{
  pick: [file: File | null]
  clear: []
}>()

const { t } = useI18n({ useScope: 'global' })
const fileInputRef = ref<HTMLInputElement | null>(null)

function onChange(e: Event) {
  const input = e.target as HTMLInputElement
  emit('pick', input.files?.[0] ?? null)
  input.value = ''
}
</script>
