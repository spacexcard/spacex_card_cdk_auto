import { createI18n } from 'vue-i18n'
import zh from './locales/zh'
import en from './locales/en'

export type AppLocale = 'zh' | 'en'

const STORAGE_KEY = 'locale'

function detectLocale(): AppLocale {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved === 'zh' || saved === 'en') return saved
  // 跟随浏览器：非中文默认英文
  return navigator.language && navigator.language.toLowerCase().startsWith('zh') ? 'zh' : 'zh'
}

const locale = detectLocale()
document.documentElement.lang = locale === 'zh' ? 'zh-CN' : 'en'

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale,
  fallbackLocale: 'zh',
  messages: { zh, en },
})

export function setLocale(next: AppLocale) {
  i18n.global.locale.value = next
  localStorage.setItem(STORAGE_KEY, next)
  document.documentElement.lang = next === 'zh' ? 'zh-CN' : 'en'
}
