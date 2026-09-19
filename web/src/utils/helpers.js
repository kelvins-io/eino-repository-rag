import { i18n } from '@/i18n'

const SESSION_KEY = 'eino_rag_session_id'

export const EMPTY_ANSWER = '__EMPTY_ANSWER__'

export function getSessionId() {
  let id = localStorage.getItem(SESSION_KEY)
  if (!id) {
    id = `s-${Date.now().toString(36)}`
    localStorage.setItem(SESSION_KEY, id)
  }
  return id
}

export function setSessionId(id) {
  if (id) localStorage.setItem(SESSION_KEY, id)
}

export function newSessionId() {
  const id = `s-${Date.now().toString(36)}`
  localStorage.setItem(SESSION_KEY, id)
  return id
}

export function formatSize(bytes) {
  if (!bytes && bytes !== 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

export function formatTime(v) {
  if (!v) return '-'
  const locale = i18n.global.locale.value === 'en' ? 'en-US' : 'zh-CN'
  return new Date(v).toLocaleString(locale)
}

export function statusType(status) {
  const map = {
    pending: 'info',
    indexing: 'warning',
    ready: 'success',
    failed: 'danger',
  }
  return map[status] || 'info'
}

export function statusLabel(status) {
  const key = `status.${status}`
  return i18n.global.te(key) ? i18n.global.t(key) : status
}
