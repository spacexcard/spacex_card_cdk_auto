<template>
  <div class="space-y-4">
    <el-card shadow="never">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">站点外观与配置</span>
          <el-tag size="small" type="success">DB 持久化</el-tag>
        </div>
      </template>
      <p class="text-sm text-muted mb-4">
        品牌/皮肤写入服务端 <code>site_settings</code>。
      </p>

      <el-form label-width="120px" class="max-w-xl">
        <el-form-item label="站点名称">
          <el-input v-model="form.brand_name" maxlength="40" show-word-limit />
        </el-form-item>
        <el-form-item label="副标题">
          <el-input v-model="form.brand_sub" maxlength="80" show-word-limit />
        </el-form-item>
        <el-form-item label="整站主题">
          <div class="w-full">
            <SkinPicker show-mode title="主题包（配色 + 字体 + 布局）" />
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveBrand">保存品牌与主题</el-button>
          <el-button @click="reapply">强制重应用主题</el-button>
        </el-form-item>
        <p class="text-xs text-subtle -mt-2">
          主题切换会同时改配色、字体、圆角、导航与布局壳。若偶尔「半套」，点「强制重应用」。
          推荐试：<b>赛博科技</b>（侧栏+网格）、石板极简（细侧轨）、墨黑工坊。
        </p>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { siteBrand, setSiteBrand, setSkin, setTheme, siteSkin, themeMode, applyThemeAll, type SkinId, type ThemeMode } from '../../theme'
import SkinPicker from '../../components/SkinPicker.vue'
import { authFetch } from '../../lib/api'
import { dialog } from '../../lib/dialog'

const form = reactive({
  brand_name: siteBrand.value.name,
  brand_sub: siteBrand.value.sub,
})
const saving = ref(false)

onMounted(async () => {
  try {
    const r = await authFetch('/api/v1/admin/settings')
    if (!r.ok) return
    const d = await r.json()
    form.brand_name = d.brand_name || form.brand_name
    form.brand_sub = d.brand_sub || form.brand_sub
    if (d.skin) setSkin(d.skin as SkinId)
    if (d.theme_mode) setTheme(d.theme_mode as ThemeMode)
    setSiteBrand({ name: form.brand_name, sub: form.brand_sub })
  } catch { /* ignore */ }
})

function reapply() {
  applyThemeAll()
  dialog.toast('主题已重应用', 'ok')
}

async function saveBrand() {
  saving.value = true
  try {
    setSiteBrand({ name: form.brand_name, sub: form.brand_sub })
    applyThemeAll()
    const r = await authFetch('/api/v1/admin/settings', {
      method: 'PUT',
      body: JSON.stringify({
        brand_name: form.brand_name,
        brand_sub: form.brand_sub,
        skin: siteSkin.value,
        theme_mode: themeMode.value,
      }),
    })
    if (!r.ok) {
      const d = await r.json().catch(() => ({}))
      dialog.toast(d.error || '保存失败', 'err')
      return
    }
    dialog.toast('品牌/主题已保存', 'ok')
  } finally {
    saving.value = false
  }
}
</script>
