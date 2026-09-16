const USER_KEY = 'eino_rag_user_id'
const SESSION_KEY = 'eino_rag_session_id'

export function getUserId() {
  let id = localStorage.getItem(USER_KEY)
  if (!id) {
    id = 'u001'
    localStorage.setItem(USER_KEY, id)
  }
  return id
}

export function setUserId(id) {
  localStorage.setItem(USER_KEY, id || 'u001')
}

export function getSessionId() {
  let id = localStorage.getItem(SESSION_KEY)
  if (!id) {
    id = `s-${Date.now().toString(36)}`
    localStorage.setItem(SESSION_KEY, id)
  }
  return id
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
  const map = {
    pending: '待索引',
    indexing: '索引中',
    ready: '就绪',
    failed: '失败',
  }
  return map[status] || status
}
