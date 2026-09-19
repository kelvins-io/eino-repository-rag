import { createI18n } from 'vue-i18n'
import zhCN from './locales/zh-CN.json'
import en from './locales/en.json'

export const LOCALE_KEY = 'eino_rag_locale'
export const LOCALES = [
  { value: 'zh-CN', label: '中文' },
  { value: 'en', label: 'English' },
]

function readLocale() {
  try {
    const saved = localStorage.getItem(LOCALE_KEY)
    if (saved === 'zh-CN' || saved === 'en') return saved
  } catch {
    /* ignore */
  }
  const nav = (navigator.language || '').toLowerCase()
  if (nav.startsWith('zh')) return 'zh-CN'
  if (nav.startsWith('en')) return 'en'
  return 'zh-CN'
}

export const i18n = createI18n({
  legacy: false,
  locale: readLocale(),
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    en,
  },
})

export function t(key, params) {
  return i18n.global.t(key, params || {})
}

export function setAppLocale(locale) {
  if (locale !== 'zh-CN' && locale !== 'en') return
  i18n.global.locale.value = locale
  try {
    localStorage.setItem(LOCALE_KEY, locale)
  } catch {
    /* ignore */
  }
  document.documentElement.lang = locale
}

document.documentElement.lang = i18n.global.locale.value
