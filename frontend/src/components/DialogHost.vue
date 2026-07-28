<template>
  <!-- Toast 队列 -->
  <div class="fixed top-4 right-4 z-[9999] flex flex-col gap-2 pointer-events-none">
    <transition-group name="toast">
      <div v-for="t in dialogState.toasts" :key="t.id"
        class="pointer-events-auto flex items-center gap-2 px-4 py-2.5 rounded-lg text-sm text-white shadow-lg max-w-sm break-all"
        :class="toastBg(t.type)">
        <span class="font-bold">{{ t.type === 'err' ? '✕' : t.type === 'warn' ? '!' : t.type === 'info' ? 'i' : '✓' }}</span>
        <span>{{ t.message }}</span>
      </div>
    </transition-group>
  </div>

  <!-- 弹窗 -->
  <div v-if="d" class="fixed inset-0 z-[9998] flex items-center justify-center bg-black/50 p-4" @click.self="onCancel">
    <div class="card w-full max-w-md" @keydown.enter="onEnter" @keydown.esc="onCancel">
      <h3 class="text-lg font-semibold text-ink mb-2">{{ d.title }}</h3>
      <div v-if="d.message" class="text-sm text-muted whitespace-pre-line mb-4">{{ d.message }}</div>

      <input v-if="d.kind === 'prompt'" ref="inputEl" v-model="d.value" :placeholder="d.placeholder"
        class="input w-full mb-4" @keydown.enter.prevent="onEnter" />

      <div v-if="d.kind === 'select'" class="flex flex-col gap-2 mb-4">
        <button v-for="(o, i) in d.options" :key="i"
          class="text-left px-4 py-3 rounded-lg border border-black/10 dark:border-white/10 hover:border-indigo-500 hover:bg-indigo-500/5 transition"
          @click="resolveActive(o.value)">
          <div class="text-ink font-medium">{{ o.label }}</div>
          <div v-if="o.desc" class="text-xs text-muted mt-0.5">{{ o.desc }}</div>
        </button>
      </div>

      <div class="flex justify-end gap-2">
        <button v-if="d.kind !== 'alert'" class="btn-secondary" @click="onCancel">{{ d.cancelText || '取消' }}</button>
        <button v-if="d.kind === 'alert' || d.kind === 'confirm' || d.kind === 'prompt'"
          :class="d.danger ? 'btn-danger' : 'btn-primary'" @click="onEnter">{{ d.okText || '确定' }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { dialogState, resolveActive } from '../lib/dialog'

const d = computed(() => dialogState.active)
const inputEl = ref<HTMLInputElement | null>(null)

watch(d, async (val) => {
  if (val && val.kind === 'prompt') {
    await nextTick()
    inputEl.value?.focus()
    inputEl.value?.select()
  }
})

function toastBg(type: string) {
  return type === 'err' ? 'bg-red-600' : type === 'warn' ? 'bg-amber-600' : type === 'info' ? 'bg-blue-600' : 'bg-emerald-600'
}
function onEnter() {
  const a = d.value
  if (!a) return
  if (a.kind === 'alert' || a.kind === 'confirm') resolveActive(true)
  else if (a.kind === 'prompt') resolveActive(a.value)
}
function onCancel() {
  const a = d.value
  if (!a) return
  resolveActive(a.kind === 'confirm' || a.kind === 'alert' ? false : null)
}
</script>

<style scoped>
.toast-enter-active, .toast-leave-active { transition: all .25s ease; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateX(20px); }
</style>
