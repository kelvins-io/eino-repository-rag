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

  // 问答
  chatQuery: (data) => http.post('/api/v1/chat/query', data),
  chatHistory: (sessionId) =>
    http.get('/api/v1/chat/history', { params: { session_id: sessionId } }),
}

export default http
