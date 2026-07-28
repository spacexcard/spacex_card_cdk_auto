<template>
  <div class="skin-picker" :class="{ compact }">
    <div v-if="!compact" class="sp-head">
      <div class="sp-title">{{ title || '整站主题' }}</div>
      <div class="sp-sub">切换将改变配色、字体、圆角与导航形态</div>
    </div>
    <div class="sp-grid">
      <button
        v-for="s in SKINS"
        :key="s.id"
        type="button"
        class="sp-item"
        :class="{ active: siteSkin === s.id }"
        @click="setSkin(s.id)"
      >
        <span class="sp-swatches">
          <i :style="{ background: s.swatch }" />
          <i :style="{ background: s.swatch2 }" />
        </span>
        <span class="sp-meta">
          <span class="sp-label">{{ s.label }}</span>
          <span v-if="!compact" class="sp-blurb">{{ s.blurb }}</span>
        </span>
      </button>
    </div>
    <div v-if="showMode" class="sp-mode">
      <span class="sp-mode-label">明暗叠加</span>
      <div class="sp-mode-btns">
        <button type="button" :class="{ on: themeMode === 'light' }" @click="setTheme('light')">亮</button>
        <button type="button" :class="{ on: themeMode === 'dark' }" @click="setTheme('dark')">暗</button>
        <button type="button" :class="{ on: themeMode === 'auto' }" @click="setTheme('auto')">自动</button>
      </div>
    </div>
    <div v-if="!compact && current" class="sp-current">
      当前：<b>{{ current.label }}</b> · {{ current.heading }} / {{ current.density }} / {{ current.nav }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { SKINS, siteSkin, setSkin, themeMode, setTheme, currentSkinMeta } from '../theme'

defineProps<{
  compact?: boolean
  showMode?: boolean
  title?: string
}>()

const current = computed(() => currentSkinMeta.value)
</script>

<style scoped>
.skin-picker { display: flex; flex-direction: column; gap: 12px; }
.sp-head { display: flex; flex-direction: column; gap: 2px; }
.sp-title { font-size: 14px; font-weight: 700; color: var(--ink); font-family: var(--font-display, var(--font-serif)); }
.sp-sub { font-size: 11px; color: var(--ink-3); line-height: 1.4; }
.sp-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
  max-height: 360px;
  overflow: auto;
  padding-right: 2px;
}
.compact .sp-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
  max-height: none;
}
.sp-item {
  display: flex; align-items: center; gap: 10px;
  padding: 10px 12px; border-radius: var(--radius-md, 12px);
  border: 1px solid var(--brd);
  background: var(--surface); cursor: pointer; transition: .18s ease;
  text-align: left;
}
.sp-item:hover { border-color: var(--primary); transform: translateY(-1px); }
.sp-item.active {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px var(--primary-soft);
  background: var(--primary-soft);
}
.sp-swatches {
  display: flex; width: 36px; height: 36px; border-radius: 10px; overflow: hidden;
  border: 1px solid var(--brd); flex-shrink: 0;
}
.sp-swatches i { flex: 1; display: block; }
.sp-meta { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.sp-label { font-size: 13px; font-weight: 600; color: var(--ink); }
.sp-blurb { font-size: 11px; color: var(--ink-3); line-height: 1.35; }
.compact .sp-item { flex-direction: column; padding: 8px 6px; gap: 6px; }
.compact .sp-blurb { display: none; }
.compact .sp-swatches { width: 100%; height: 22px; border-radius: 6px; }
.sp-mode { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.sp-mode-label { font-size: 12px; color: var(--ink-3); }
.sp-mode-btns { display: flex; gap: 4px; }
.sp-mode-btns button {
  font-size: 12px; padding: 5px 12px; border-radius: 999px; border: 1px solid var(--brd);
  background: var(--surface-2); color: var(--ink-2); cursor: pointer;
}
.sp-mode-btns button.on { background: var(--primary); color: var(--primary-on); border-color: transparent; }
.sp-current { font-size: 11px; color: var(--ink-3); padding-top: 4px; border-top: 1px dashed var(--brd); }
</style>
