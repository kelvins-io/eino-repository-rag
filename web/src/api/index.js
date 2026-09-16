import axios from 'axios'
import { ElMessage } from 'element-plus'

const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '',
  timeout: 120000,
})

http.interceptors.response.use(
  (res) => {
    const body = res.data
    if (body && typeof body.code === 'number' && body.code !== 0) {
      const msg = body.message || '请求失败'
      ElMessage.error(msg)
      return Promise.reject(new Error(msg))
    }
    return body?.data !== undefined ? body.data : body
  },
  (err) => {
    const msg =
      err.response?.data?.message ||
      err.message ||
      '网络错误'
    ElMessage.error(msg)
    return Promise.reject(err)
  },
)

/**
 * 消费 POST /api/v1/chat/query 的 SSE 流。
 * 事件: meta | delta | done | error
 */
export async function chatQueryStream(data, handlers = {}, signal) {
  const base = import.meta.env.VITE_API_BASE || ''
  const res = await fetch(`${base}/api/v1/chat/query`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    },
    body: JSON.stringify(data),
    signal,
  })

  if (!res.ok) {
    let msg = `请求失败 (${res.status})`
    try {
      const body = await res.json()
      if (body?.message) msg = body.message
    } catch {
      /* ignore */
    }
    throw new Error(msg)
  }

  if (!res.body) {
    throw new Error('浏览器不支持流式响应')
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''
  let doneEvent = null

  const dispatch = (evt) => {
    if (!evt || !evt.type) return
    switch (evt.type) {
      case 'meta':
        handlers.onMeta?.(evt)
        break
      case 'delta':
        handlers.onDelta?.(evt.content || '')
        break
      case 'done':
        doneEvent = evt
        handlers.onDone?.(evt)
        break
      case 'error':
        handlers.onError?.(evt.message || '流式回答失败')
        throw new Error(evt.message || '流式回答失败')
      default:
        break
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })

    let sep
    while ((sep = buffer.indexOf('\n\n')) >= 0) {
      const raw = buffer.slice(0, sep)
      buffer = buffer.slice(sep + 2)
      const lines = raw.split('\n')
      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed.startsWith('data:')) continue
        const payload = trimmed.slice(5).trim()
        if (!payload || payload === '[DONE]') continue
        let evt
        try {
          evt = JSON.parse(payload)
        } catch {
          continue
        }
        dispatch(evt)
      }
    }
  }

  return doneEvent
}

export const api = {
  health: () => http.get('/health'),

  // 知识库
  listKnowledgeBases: (userId) =>
    http.get('/api/v1/knowledge-bases', { params: { user_id: userId } }),
  getKnowledgeBase: (id) => http.get(`/api/v1/knowledge-bases/${id}`),
  createKnowledgeBase: (data) => http.post('/api/v1/knowledge-bases', data),
  updateKnowledgeBase: (id, data) => http.put(`/api/v1/knowledge-bases/${id}`, data),
  deleteKnowledgeBase: (id) => http.delete(`/api/v1/knowledge-bases/${id}`),

  // 目录
  listDirectories: (kbId) => http.get(`/api/v1/knowledge-bases/${kbId}/directories`),
  createDirectory: (kbId, data) =>
    http.post(`/api/v1/knowledge-bases/${kbId}/directories`, data),
  updateDirectory: (id, data) => http.put(`/api/v1/directories/${id}`, data),
  deleteDirectory: (id) => http.delete(`/api/v1/directories/${id}`),

  // 文档
  listDocuments: (params) => http.get('/api/v1/documents', { params }),
  getDocument: (id) => http.get(`/api/v1/documents/${id}`),
  deleteDocument: (id) => http.delete(`/api/v1/documents/${id}`),
  deleteDocuments: (ids) => http.post('/api/v1/documents/delete', { ids }),
  reindexDocuments: (ids) => http.post('/api/v1/documents/reindex', { ids }),
  importDocuments: (formData) =>
    http.post('/api/v1/documents/import', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    }),

  // 问答（SSE 流式）
  chatQueryStream,
  chatHistory: (sessionId) =>
    http.get('/api/v1/chat/history', { params: { session_id: sessionId } }),
}

export default http
