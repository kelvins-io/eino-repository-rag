<template>
  <div class="chat-page">
    <div class="page-header">
      <div>
        <h2>知识库问答</h2>
        <p class="sub">基于 RAG 检索 + 会话记忆回答问题；可选 Agent 多步检索</p>
      </div>
      <el-button :disabled="sessionRemaining === 0" @click="resetSession">{{ newSessionLabel }}</el-button>
    </div>

    <el-row :gutter="16">
      <el-col :span="6">
        <div class="panel side">
          <el-form label-position="top">
            <el-form-item label="问答模式">
              <el-radio-group v-model="chatMode" size="small">
                <el-radio-button value="rag">标准 RAG</el-radio-button>
                <el-radio-button value="agent">Agent</el-radio-button>
              </el-radio-group>
              <div class="session-hint">
                Agent 可多轮调用知识库检索，延迟与费用更高。
              </div>
            </el-form-item>
            <el-form-item label="知识库">
              <el-select
                v-model="kbId"
                placeholder="选择知识库"
                style="width: 100%"
                @change="onKbChange"
              >
                <el-option
                  v-for="kb in kbs"
                  :key="kb.id"
                  :label="kb.name"
                  :value="kb.id"
                />
              </el-select>
            </el-form-item>
            <el-form-item label="目录过滤（可选）">
              <el-tree-select
                v-model="directoryId"
                :data="treeData"
                clearable
                check-strictly
                node-key="id"
                :props="{ label: 'name', children: 'children', value: 'id' }"
                placeholder="不选则检索整个知识库"
                style="width: 100%"
                @change="onDirectoryChange"
              />
            </el-form-item>
            <el-form-item label="Session ID">
              <el-select
                v-model="sessionId"
                filterable
                allow-create
                default-first-option
                placeholder="选择或筛选历史会话"
                style="width: 100%"
                :loading="sessionsLoading"
                @change="onSessionChange"
              >
                <el-option
                  v-for="s in sessions"
                  :key="s.session_id"
                  :label="sessionLabel(s)"
                  :value="s.session_id"
                >
                  <div class="session-option">
                    <div class="session-id">{{ s.session_id }}</div>
                    <div class="session-meta">
                      <span>{{ s.title || '未命名会话' }}</span>
                      <span>{{ formatTime(s.updated_at) }}</span>
                    </div>
                  </div>
                </el-option>
              </el-select>
              <div class="session-hint">
                展示当前租户/用户在所选知识库
                {{ directoryId ? '与目录' : '（未选目录）' }}
                下的历史会话，可筛选切换。
              </div>
            </el-form-item>
          </el-form>
        </div>
      </el-col>

      <el-col :span="18">
        <div class="panel chat-panel">
          <div ref="listRef" class="messages">
            <div v-if="!messages.length" class="empty">
              选择知识库后开始提问，同一会话会保留对话记忆。
            </div>
            <div
              v-for="(m, idx) in messages"
              :key="idx"
              class="msg-row"
              :class="m.role"
            >
              <div class="chat-bubble" :class="m.role === 'user' ? 'user' : 'assistant'">
                {{ m.content }}
              </div>
              <div v-if="showActions(m, idx)" class="msg-actions">
                <el-button
                  v-if="canSpeak(m, idx)"
                  class="tts-btn"
                  text
                  size="small"
                  :loading="loadingIdx === idx"
                  :type="speakingIdx === idx ? 'primary' : ''"
                  :disabled="ttsRemaining === 0 && speakingIdx !== idx && loadingIdx !== idx"
                  :aria-label="ttsButtonLabel(idx)"
                  @click="speak(idx, m.content)"
                >
                  <el-icon>
                    <VideoPause v-if="speakingIdx === idx" />
                    <VideoPlay v-else />
                  </el-icon>
                  {{ speakingIdx === idx ? '停止朗读' : '朗读' }}
                </el-button>
                <div v-if="canFeedback(m, idx)" class="feedback">
                  <el-button
                    text
                    size="small"
                    :type="m.vote === 'up' ? 'primary' : ''"
                    :disabled="feedbackId === m.id"
                    aria-label="点赞"
                    @click="setVote(m, 'up')"
                  >
                    {{ m.vote === 'up' ? '已赞' : '赞' }}
                  </el-button>
                  <el-button
                    text
                    size="small"
                    :type="m.vote === 'down' ? 'danger' : ''"
                    :disabled="feedbackId === m.id"
                    aria-label="点踩"
                    @click="setVote(m, 'down')"
                  >
                    {{ m.vote === 'down' ? '已踩' : '踩' }}
                  </el-button>
                  <span class="feedback-label">评分</span>
                  <el-rate
                    class="feedback-rate"
                    :model-value="m.score || 0"
                    :disabled="feedbackId === m.id"
                    clearable
                    aria-label="评分"
                    @change="(val) => setScore(m, val)"
                  />
                </div>
              </div>
              <div v-if="m.steps?.length" class="agent-steps">
                <div
                  v-for="(s, si) in m.steps"
                  :key="si"
                  class="agent-step"
                >
                  <span class="step-badge">{{ s.kind }}</span>
                  <span>{{ s.text }}</span>
                </div>
              </div>
              <div v-if="m.sources?.length" class="sources">
                <el-collapse>
                  <el-collapse-item :name="idx">
                    <template #title>
                      <span class="sources-title">引用来源（{{ m.sources.length }}）</span>
                    </template>
                    <div
                      v-for="(s, i) in m.sources"
                      :key="s.id || i"
                      class="source-item"
                    >
                      <div class="source-head">
                        <strong>[{{ i + 1 }}] {{ s.title || `来源 ${i + 1}` }}</strong>
                        <span class="source-meta">
                          <template v-if="s.doc_id">doc_id={{ s.doc_id }}</template>
                          <template v-if="s.page"> · 第 {{ s.page }} 页</template>
                          <template v-if="s.chunk_index != null"> · chunk={{ s.chunk_index }}</template>
                          <template v-if="s.score != null"> · score={{ Number(s.score).toFixed(3) }}</template>
                        </span>
                      </div>
                      <div class="source-body">{{ s.content }}</div>
                    </div>
                  </el-collapse-item>
                </el-collapse>
              </div>
              <div v-if="canLabel(m, idx)" class="relevance">
                <span class="feedback-label">相关文档</span>
                <el-select
                  :model-value="questionOf(idx).relevant_doc_ids"
                  multiple
                  filterable
                  collapse-tags
                  collapse-tags-tooltip
                  placeholder="选择这条问题应召回的文档"
                  class="relevance-select"
                  :disabled="labelingId === questionOf(idx).id"
                  @change="(val) => setQuestionDocs(idx, val)"
                >
                  <el-option
                    v-for="d in kbDocs"
                    :key="d.id"
                    :label="docOptionLabel(d)"
                    :value="String(d.id)"
                  />
                </el-select>
                <el-button
                  size="small"
                  type="primary"
                  plain
                  :loading="labelingId === questionOf(idx).id"
                  @click="saveRelevance(questionOf(idx))"
                >
                  保存标注
                </el-button>
              </div>
            </div>
          </div>

          <div class="composer">
            <div class="composer-input">
              <el-input
                v-model="query"
                type="textarea"
                :rows="3"
                :placeholder="speechPlaceholder"
                @keydown="onKeydown"
              />
              <div v-if="listening || transcribing" class="speech-live">
                <span class="speech-dot" />
                <span class="speech-live-text">
                  {{ statusText || (transcribing ? '正在识别，请稍候…' : '正在录音，请开始说话…') }}
                </span>
                <span v-if="listening" class="speech-live-hint">5 秒无声音将自动结束</span>
              </div>
            </div>
            <div class="composer-actions">
              <el-tooltip :content="speechTip" placement="top">
                <span class="speech-btn-wrap">
                  <el-button
                    class="speech-btn"
                    :class="{ 'is-listening': listening }"
                    :type="listening ? 'danger' : 'default'"
                    :loading="transcribing"
                    :disabled="asking || transcribing || !speechSupported || voiceRemaining === 0"
                    :aria-label="speechAriaLabel"
                    @click="toggleSpeech"
                  >
                    <el-icon>
                      <Microphone />
                    </el-icon>
                    <span>{{ voiceButtonLabel }}</span>
                  </el-button>
                </span>
              </el-tooltip>
              <el-button
                type="primary"
                :loading="asking"
                :disabled="!kbId || !query.trim() || listening || transcribing"
                @click="ask"
              >
                发送
              </el-button>
            </div>
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Microphone, VideoPause, VideoPlay } from '@element-plus/icons-vue'
import { api } from '@/api'
import { isTenantAdmin } from '@/utils/auth'
import { useSpeechInput } from '@/composables/useSpeechInput'
import { useSpeechOutput } from '@/composables/useSpeechOutput'
import { getSessionId, newSessionId, setSessionId } from '@/utils/helpers'

const sessionId = ref(getSessionId())
const sessions = ref([])
const sessionsLoading = ref(false)
const kbs = ref([])
const kbId = ref()
const treeData = ref([])
const directoryId = ref()
const messages = ref([])
const query = ref('')
const asking = ref(false)
const feedbackId = ref(0)
const labelingId = ref(0)
const kbDocs = ref([])
const tenantAdmin = computed(() => isTenantAdmin())
const sessionRemaining = ref(null)
const voiceRemaining = ref(null)
const ttsRemaining = ref(null)
const listRef = ref()
const chatMode = ref('rag')
const {
  listening,
  transcribing,
  statusText,
  supported: speechSupported,
  toggle: toggleSpeech,
  stop: stopSpeech,
} = useSpeechInput(query, {
  getPrompt() {
    const kb = kbs.value.find((item) => item.id === kbId.value)
    return kb?.name ? `知识库问答，相关主题：${kb.name}` : '知识库问答'
  },
  onTranscribed: () => loadQuota(),
})
const {
  speakingIdx,
  loadingIdx,
  toggle: toggleSpeak,
  stop: stopSpeak,
} = useSpeechOutput()

watch(listening, (on) => {
  if (on) stopSpeak()
})

watch(transcribing, (on, prev) => {
  if (prev && !on) loadQuota()
})

const speechPlaceholder = computed(() => {
  if (transcribing.value) return '正在识别语音…'
  if (listening.value) return '正在录音，5 秒无声音将自动结束'
  return '输入问题，Enter 发送，Shift+Enter 换行'
})

const speechTip = computed(() => {
  if (voiceRemaining.value === 0) return '今日语音输入次数已用完'
  if (!speechSupported.value) return '当前浏览器不支持录音，请使用 Chrome、Edge 或 Safari'
  if (transcribing.value) return '正在识别'
  if (listening.value) return '点击停止并识别为文字'
  return '语音输入'
})

const speechAriaLabel = computed(() => {
  if (transcribing.value) return '正在识别语音'
  if (listening.value) return '停止语音输入'
  return '语音输入'
})

const newSessionLabel = computed(() => {
  if (sessionRemaining.value == null) return '新会话'
  return `新会话（今日剩余 ${sessionRemaining.value}）`
})

const voiceButtonLabel = computed(() => {
  if (voiceRemaining.value == null) return '语音输入'
  return `语音输入（今日剩余 ${voiceRemaining.value}）`
})

async function loadQuota() {
  try {
    const data = await api.chatQuota()
    sessionRemaining.value = Number(data?.remaining_sessions ?? 0)
    voiceRemaining.value = Number(data?.remaining_voice_inputs ?? 0)
    ttsRemaining.value = Number(data?.remaining_tts ?? 0)
  } catch {
    /* 保留上次结果，避免暂时失败把按钮锁死 */
  }
}

function formatTime(v) {
  if (!v) return ''
  return new Date(v).toLocaleString()
}

function sessionLabel(s) {
  const title = s.title || '未命名会话'
  const time = formatTime(s.updated_at)
  return time ? `${s.session_id} · ${title} · ${time}` : `${s.session_id} · ${title}`
}

async function loadKbs() {
  const data = await api.listKnowledgeBases({ page: 1, page_size: 100 })
  kbs.value = data?.list || []
  if (kbs.value.length && !kbId.value) {
    kbId.value = kbs.value[0].id
    await onKbChange()
  }
}

async function onKbChange() {
  directoryId.value = undefined
  treeData.value = kbId.value
    ? (await api.listDirectories(kbId.value)) || []
    : []
  await Promise.all([reloadSessionsAndHistory(), loadKbDocs()])
}

async function onDirectoryChange() {
  await reloadSessionsAndHistory()
}

async function loadSessions() {
  if (!kbId.value) {
    sessions.value = []
    return
  }
  sessionsLoading.value = true
  try {
    const params = { knowledge_base_id: kbId.value }
    if (directoryId.value) {
      params.directory_id = directoryId.value
    }
    sessions.value = (await api.listChatSessions(params)) || []
  } catch {
    sessions.value = []
  } finally {
    sessionsLoading.value = false
  }
}

async function reloadSessionsAndHistory() {
  await loadSessions()
  const exists = sessions.value.some((s) => s.session_id === sessionId.value)
  if (!exists) {
    sessionId.value = newSessionId()
    messages.value = []
    return
  }
  setSessionId(sessionId.value)
  await loadHistory()
}

async function loadHistory() {
  if (!sessionId.value) {
    messages.value = []
    return
  }
  try {
    const list = await api.chatHistory(sessionId.value)
    messages.value = (list || []).map((m) => ({
      id: m.id,
      role: m.role,
      content: m.content,
      vote: m.vote || '',
      score: m.score || 0,
      relevant_doc_ids: Array.isArray(m.relevant_doc_ids) ? m.relevant_doc_ids.map(String) : [],
    }))
    await scrollBottom()
  } catch {
    messages.value = []
  }
}

async function onSessionChange(id) {
  if (!id) return
  stopSpeak()
  setSessionId(id)
  sessionId.value = id
  await loadHistory()
}

function resetSession() {
  if (sessionRemaining.value === 0) return
  stopSpeech({ commit: false })
  stopSpeak()
  sessionId.value = newSessionId()
  messages.value = []
  ElMessage.success('已开始新会话')
}

async function scrollBottom() {
  await nextTick()
  if (listRef.value) {
    listRef.value.scrollTop = listRef.value.scrollHeight
  }
}

function onKeydown(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (!listening.value && !transcribing.value) ask()
  }
}

function canFeedback(m, idx) {
  if (m.role !== 'assistant' || !m.id) return false
  if (asking.value && idx === messages.value.length - 1) return false
  return true
}

function showActions(m, idx) {
  return canSpeak(m, idx) || canFeedback(m, idx)
}

function questionOf(idx) {
  for (let i = idx - 1; i >= 0; i -= 1) {
    if (messages.value[i]?.role === 'user') return messages.value[i]
  }
  return null
}

function canLabel(m, idx) {
  if (!tenantAdmin.value || m.role !== 'assistant') return false
  if (asking.value && idx === messages.value.length - 1) return false
  return !!questionOf(idx)?.id
}

function docOptionLabel(d) {
  const name = d.title || d.file_name || `文档 ${d.id}`
  return `${name} (#${d.id})`
}

function setQuestionDocs(idx, ids) {
  const q = questionOf(idx)
  if (!q) return
  q.relevant_doc_ids = (ids || []).map(String)
}

async function loadKbDocs() {
  if (!tenantAdmin.value || !kbId.value) {
    kbDocs.value = []
    return
  }
  try {
    const all = []
    let page = 1
    let total = 0
    do {
      const data = await api.listDocuments({
        knowledge_base_id: kbId.value,
        page,
        page_size: 100,
      })
      const list = data?.list || []
      total = Number(data?.total || 0)
      all.push(...list)
      if (!list.length) break
      page += 1
    } while (all.length < total && page <= 5)
    kbDocs.value = all
  } catch {
    kbDocs.value = []
  }
}

async function saveRelevance(question) {
  if (!question?.id || labelingId.value) return
  const prev = [...(question.relevant_doc_ids || [])]
  labelingId.value = question.id
  try {
    const saved = await api.setChatRelevance({
      session_id: sessionId.value,
      message_id: question.id,
      doc_ids: prev,
    })
    question.relevant_doc_ids = saved?.doc_ids || []
    ElMessage.success('已保存相关文档标注')
  } catch {
    question.relevant_doc_ids = prev
  } finally {
    labelingId.value = 0
  }
}

async function setVote(m, vote) {
  if (!m?.id || feedbackId.value) return
  const next = m.vote === vote ? '' : vote
  const prev = m.vote || ''
  feedbackId.value = m.id
  m.vote = next
  try {
    const saved = await api.setChatFeedback({
      session_id: sessionId.value,
      message_id: m.id,
      vote: next,
    })
    m.vote = saved?.vote || ''
    if (saved && saved.score != null) m.score = saved.score
  } catch {
    m.vote = prev
  } finally {
    feedbackId.value = 0
  }
}

async function setScore(m, score) {
  if (!m?.id || feedbackId.value) return
  const next = Number(score) || 0
  if (next === (m.score || 0)) return
  const prev = m.score || 0
  feedbackId.value = m.id
  m.score = next
  try {
    const saved = await api.setChatFeedback({
      session_id: sessionId.value,
      message_id: m.id,
      score: next,
    })
    m.vote = saved?.vote || ''
    m.score = saved?.score || 0
  } catch {
    m.score = prev
  } finally {
    feedbackId.value = 0
  }
}

function canSpeak(m, idx) {
  if (m.role !== 'assistant') return false
  const text = (m.content || '').trim()
  if (!text || text === '(空回答)') return false
  if (asking.value && idx === messages.value.length - 1) return false
  return true
}

function ttsButtonLabel(idx) {
  if (speakingIdx.value === idx) return '停止朗读'
  if (ttsRemaining.value === 0) return '今日文字转语音次数已用完'
  return '朗读回答'
}

async function speak(idx, text) {
  await toggleSpeak(idx, text)
  await loadQuota()
}

async function ask() {
  const q = query.value.trim()
  if (!q || !kbId.value || asking.value) return
  stopSpeak()

  messages.value.push({ role: 'user', content: q, relevant_doc_ids: [] })
  query.value = ''
  messages.value.push({ role: 'assistant', content: '', sources: [], steps: [] })
  const assistantIdx = messages.value.length - 1
  await scrollBottom()

  asking.value = true
  try {
    const payload = {
      session_id: sessionId.value,
      knowledge_base_id: kbId.value,
      query: q,
    }
    if (directoryId.value) {
      payload.directory_id = directoryId.value
    }
    const streamFn =
      chatMode.value === 'agent' ? api.chatAgentStream : api.chatQueryStream
    const pushStep = (kind, text) => {
      if (!messages.value[assistantIdx].steps) {
        messages.value[assistantIdx].steps = []
      }
      messages.value[assistantIdx].steps.push({ kind, text })
    }
    await streamFn(payload, {
      onMeta: (evt) => {
        if (evt.session_id) {
          sessionId.value = evt.session_id
          setSessionId(evt.session_id)
        }
        if (evt.sources?.length) {
          messages.value[assistantIdx].sources = evt.sources
        }
      },
      onStep: (evt) => {
        pushStep('step', evt.message || `步骤 ${evt.step}`)
      },
      onToolStart: (evt) => {
        const qText = evt.tool_query ? `：${evt.tool_query}` : ''
        pushStep('tool', `检索中${qText}`)
      },
      onToolResult: (evt) => {
        pushStep('result', `检索完成（${evt.tool_count ?? 0} 条）`)
      },
      onDelta: async (chunk) => {
        messages.value[assistantIdx].content += chunk
        await scrollBottom()
      },
      onDone: async (evt) => {
        if (evt.session_id) {
          sessionId.value = evt.session_id
          setSessionId(evt.session_id)
        }
        if (evt.answer) messages.value[assistantIdx].content = evt.answer
        if (evt.message_id) messages.value[assistantIdx].id = evt.message_id
        if (evt.user_message_id && messages.value[assistantIdx - 1]?.role === 'user') {
          messages.value[assistantIdx - 1].id = evt.user_message_id
        }
        if (evt.sources?.length) {
          messages.value[assistantIdx].sources = evt.sources
        }
        if (!messages.value[assistantIdx].content) {
          messages.value[assistantIdx].content = '(空回答)'
        }
        await loadSessions()
        await scrollBottom()
      },
    })
  } catch (err) {
    const msg = err?.message || '流式回答失败'
    if (!messages.value[assistantIdx].content) {
      messages.value[assistantIdx].content = msg
    }
    ElMessage.error(msg)
  } finally {
    asking.value = false
    await loadQuota()
  }
}

onMounted(async () => {
  await Promise.all([loadKbs(), loadQuota()])
})
</script>

<style scoped>
.side {
  min-height: 560px;
}

.session-hint {
  margin-top: 6px;
  font-size: 12px;
  color: #94a3b8;
  line-height: 1.4;
}

.session-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 2px 0;
  line-height: 1.3;
}

.session-id {
  font-size: 13px;
}

.session-meta {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  color: #94a3b8;
}

.chat-panel {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 160px);
  min-height: 560px;
  padding: 0;
  overflow: hidden;
}

.messages {
  flex: 1;
  overflow: auto;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  background:
    radial-gradient(circle at top left, rgba(37, 99, 235, 0.06), transparent 40%),
    #f8fafc;
}

.empty {
  margin: auto;
  color: #94a3b8;
}

.msg-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.msg-row.user {
  align-items: flex-end;
}

.msg-row.assistant {
  align-items: flex-start;
}

.msg-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 8px;
  max-width: 780px;
}

.feedback {
  display: flex;
  align-items: center;
  gap: 2px;
}

.feedback-label {
  margin-left: 4px;
  font-size: 12px;
  color: #94a3b8;
}

.feedback-rate {
  height: 24px;
}

.feedback-rate :deep(.el-rate) {
  height: 24px;
}

.feedback-rate :deep(.el-rate__icon) {
  font-size: 16px;
  margin-right: 2px;
}

.relevance {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  max-width: 780px;
}

.relevance-select {
  width: 320px;
  max-width: 100%;
}

.tts-btn {
  margin-left: -8px;
  color: #64748b;
  padding: 0 8px;
  height: 28px;
}

.tts-btn :deep(.el-icon) {
  margin-right: 4px;
}

.sources {
  max-width: 780px;
  width: 100%;
}

.sources :deep(.el-collapse) {
  border: none;
  background: transparent;
}

.sources :deep(.el-collapse-item__header) {
  height: auto;
  line-height: 1.4;
  padding: 4px 0;
  background: transparent;
  border: none;
  color: #64748b;
  font-size: 13px;
}

.sources :deep(.el-collapse-item__wrap) {
  border: none;
  background: transparent;
}

.sources :deep(.el-collapse-item__content) {
  padding: 4px 0 0;
}

.sources-title {
  font-weight: 500;
}

.agent-steps {
  margin: 6px 0 4px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.agent-step {
  font-size: 12px;
  color: #64748b;
  display: flex;
  align-items: flex-start;
  gap: 6px;
  line-height: 1.4;
}

.step-badge {
  flex-shrink: 0;
  font-size: 11px;
  padding: 0 6px;
  border-radius: 4px;
  background: #e2e8f0;
  color: #475569;
}

.composer {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  padding: 14px 16px;
  border-top: 1px solid #e2e8f0;
  background: #fff;
}

.composer-input {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.composer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.speech-btn-wrap {
  display: inline-flex;
}

.speech-btn.is-listening {
  animation: speech-pulse 1.2s ease-in-out infinite;
}

.speech-live {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 22px;
  font-size: 13px;
  color: #b91c1c;
  line-height: 1.4;
}

.speech-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #ef4444;
  flex-shrink: 0;
  animation: speech-dot 1s ease-in-out infinite;
}

.speech-live-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.speech-live-hint {
  color: #94a3b8;
  font-size: 12px;
  flex-shrink: 0;
}

@keyframes speech-pulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.45);
  }
  50% {
    box-shadow: 0 0 0 7px rgba(239, 68, 68, 0);
  }
}

@keyframes speech-dot {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
}
</style>
