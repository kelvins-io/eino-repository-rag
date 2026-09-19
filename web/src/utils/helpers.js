const SESSION_KEY = 'eino_rag_session_id'

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
  return new Date(v).toLocaleString()
}

export function statusType(status) {
  const map = {
    pending: 'gray',
    indexing: 'orange',
    ready: 'green',
    failed: 'red',
  }
  return map[status] || 'gray'
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
