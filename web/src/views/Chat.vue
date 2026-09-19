<template>
  <div class="chat-page">
    <div class="page-header">
      <div>
        <h2>{{ t('chat.title') }}</h2>
        <p class="sub">{{ t('chat.sub') }}</p>
      </div>
      <a-button :disabled="sessionRemaining === 0" @click="resetSession">{{ newSessionLabel }}</a-button>
    </div>

    <a-row :gutter="16">
      <a-col :span="6">
        <div class="panel side">
          <a-form :model="{}" layout="vertical">
            <a-form-item :label="t('chat.mode')">
              <a-radio-group v-model="chatMode" type="button" size="small">
                <a-radio value="rag">{{ t('chat.rag') }}</a-radio>
                <a-radio value="agent">{{ t('chat.agent') }}</a-radio>
              </a-radio-group>
              <div class="session-hint">
                {{ t('chat.agentHint') }}
              </div>
            </a-form-item>
            <a-form-item :label="t('chat.knowledgeBase')">
              <a-select
                v-model="kbId"
                :placeholder="t('chat.selectKb')"
                @change="onKbChange"
              >
                <a-option
                  v-for="kb in kbs"
                  :key="kb.id"
                  :label="kb.name"
                  :value="kb.id"
                />
              </a-select>
            </a-form-item>
            <a-form-item :label="t('chat.dirFilter')">
              <a-tree-select
                v-model="directoryId"
                :data="treeData"
                allow-clear
                :field-names="{ key: 'id', title: 'name', children: 'children' }"
                :placeholder="t('chat.dirPh')"
                :tree-props="{ defaultExpandAll: true }"
                @change="onDirectoryChange"
              />
            </a-form-item>
            <a-form-item :label="t('chat.sessionId')">
              <a-select
                v-model="sessionId"
                allow-search
                allow-create
                :placeholder="t('chat.sessionPh')"
                :loading="sessionsLoading"
                @change="onSessionChange"
              >
                <a-option
                  v-for="s in sessions"
                  :key="s.session_id"
                  :label="sessionLabel(s)"
                  :value="s.session_id"
                >
                  <div class="session-option">
                    <div class="session-id">{{ s.session_id }}</div>
                    <div class="session-meta">
                      <span>{{ s.title || t('chat.untitled') }}</span>
                      <span>{{ formatTime(s.updated_at) }}</span>
                    </div>
                  </div>
                </a-option>
              </a-select>
              <div class="session-hint">
                {{ directoryId ? t('chat.sessionHintWithDir') : t('chat.sessionHintNoDir') }}
              </div>
            </a-form-item>
          </a-form>
        </div>
      </a-col>

      <a-col :span="18">
        <div class="panel chat-panel">
          <div ref="listRef" class="messages">
            <div v-if="!messages.length" class="empty">
              {{ t('chat.empty') }}
            </div>
            <div
              v-for="(m, idx) in messages"
              :key="idx"
              class="msg-row"
              :class="m.role"
            >
              <div class="chat-bubble" :class="m.role === 'user' ? 'user' : 'assistant'">
                {{ messageText(m) }}
              </div>
              <div v-if="showActions(m, idx)" class="msg-actions">
                <a-button
                  v-if="canSpeak(m, idx)"
                  class="tts-btn"
                  type="text"
                  size="small"
                  :loading="loadingIdx === idx"
                  :status="speakingIdx === idx ? 'success' : 'normal'"
                  :disabled="ttsRemaining === 0 && speakingIdx !== idx && loadingIdx !== idx"
                  :aria-label="ttsButtonLabel(idx)"
                  @click="speak(idx, m.content)"
                >
                  <icon-pause v-if="speakingIdx === idx" />
                  <icon-play-arrow v-else />
                  {{ speakingIdx === idx ? t('chat.stopSpeak') : t('chat.speak') }}
                </a-button>
                <div v-if="canFeedback(m, idx)" class="feedback">
                  <a-button
                    type="text"
                    size="small"
                    :status="m.vote === 'up' ? 'success' : 'normal'"
                    :disabled="feedbackId === m.id"
                    :aria-label="t('chat.likeAria')"
                    @click="setVote(m, 'up')"
                  >
                    {{ m.vote === 'up' ? t('chat.liked') : t('chat.like') }}
                  </a-button>
                  <a-button
                    type="text"
                    size="small"
                    :status="m.vote === 'down' ? 'danger' : 'normal'"
                    :disabled="feedbackId === m.id"
                    :aria-label="t('chat.dislikeAria')"
                    @click="setVote(m, 'down')"
                  >
                    {{ m.vote === 'down' ? t('chat.disliked') : t('chat.dislike') }}
                  </a-button>
                  <span class="feedback-label">{{ t('chat.score') }}</span>
                  <a-rate
                    class="feedback-rate"
                    :model-value="m.score || 0"
                    :disabled="feedbackId === m.id"
                    allow-clear
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
                <a-collapse :bordered="false">
                  <a-collapse-item :key="idx" :header="t('chat.sources', { count: m.sources.length })">
                    <div
                      v-for="(s, i) in m.sources"
                      :key="s.id || i"
                      class="source-item"
                    >
                      <div class="source-head">
                        <strong>[{{ i + 1 }}] {{ s.title || t('chat.sourceN', { n: i + 1 }) }}</strong>
                        <span class="source-meta">
                          <template v-if="s.doc_id">doc_id={{ s.doc_id }}</template>
                          <template v-if="s.page"> · {{ t('chat.sourcePage', { page: s.page }) }}</template>
                          <template v-if="s.chunk_index != null"> · chunk={{ s.chunk_index }}</template>
                          <template v-if="s.score != null"> · score={{ Number(s.score).toFixed(3) }}</template>
                        </span>
                      </div>
                      <div class="source-body">{{ s.content }}</div>
                    </div>
                  </a-collapse-item>
                </a-collapse>
              </div>
              <div v-if="canLabel(m, idx)" class="relevance">
                <span class="feedback-label">{{ t('chat.relevantDocs') }}</span>
                <a-select
                  :model-value="questionOf(idx).relevant_doc_ids"
                  multiple
                  allow-search
                  :max-tag-count="1"
                  :placeholder="t('chat.relevantPh')"
                  class="relevance-select"
                  :disabled="labelingId === questionOf(idx).id"
                  @change="(val) => setQuestionDocs(idx, val)"
                >
                  <a-option
                    v-for="d in kbDocs"
                    :key="d.id"
                    :label="docOptionLabel(d)"
                    :value="String(d.id)"
                  />
                </a-select>
                <a-button
                  size="small"
                  type="outline"
                  :loading="labelingId === questionOf(idx).id"
                  @click="saveRelevance(questionOf(idx))"
                >
                  {{ t('chat.saveLabels') }}
                </a-button>
              </div>
            </div>
          </div>

          <div class="composer">
            <div class="composer-input">
              <a-textarea
                v-model="query"
                :auto-size="{ minRows: 3, maxRows: 6 }"
                :placeholder="speechPlaceholder"
                @keydown="onKeydown"
              />
              <div v-if="listening || transcribing" class="speech-live">
                <span class="speech-dot" />
                <span class="speech-live-text">
                  {{ statusText || (transcribing ? t('chat.recognizingLive') : t('chat.recordingLive')) }}
                </span>
                <span v-if="listening" class="speech-live-hint">{{ t('chat.silenceHint') }}</span>
              </div>
            </div>
            <div class="composer-actions">
              <a-tooltip :content="speechTip" position="top">
                <span class="speech-btn-wrap">
                  <a-button
                    class="speech-btn"
                    :class="{ 'is-listening': listening }"
                    :status="listening ? 'danger' : 'normal'"
                    :type="listening ? 'primary' : 'secondary'"
                    :loading="transcribing"
                    :disabled="asking || transcribing || !speechSupported || voiceRemaining === 0"
                    :aria-label="speechAriaLabel"
                    @click="toggleSpeech"
                  >
                    <icon-voice />
                    <span>{{ voiceButtonLabel }}</span>
                  </a-button>
                </span>
              </a-tooltip>
              <a-button
                type="primary"
                :loading="asking"
                :disabled="!kbId || !query.trim() || listening || transcribing"
                @click="ask"
              >
                {{ t('chat.send') }}
              </a-button>
            </div>
          </div>
        </div>
      </a-col>
    </a-row>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'
import { isTenantAdmin } from '@/utils/auth'
import { useSpeechInput } from '@/composables/useSpeechInput'
import { useSpeechOutput } from '@/composables/useSpeechOutput'
import { EMPTY_ANSWER, getSessionId, newSessionId, setSessionId } from '@/utils/helpers'

const { t, locale } = useI18n()

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
    return kb?.name ? t('chat.speechPromptWithKb', { name: kb.name }) : t('chat.speechPrompt')
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
  if (transcribing.value) return t('chat.recognizing')
  if (listening.value) return t('chat.recordingPlaceholder')
  return t('chat.placeholder')
})

const speechTip = computed(() => {
  if (voiceRemaining.value === 0) return t('chat.voiceUsedUp')
  if (!speechSupported.value) return t('chat.speechUnsupported')
  if (transcribing.value) return t('chat.recognizingShort')
  if (listening.value) return t('chat.stopAndTranscribe')
  return t('chat.voiceInput')
})

const speechAriaLabel = computed(() => {
  if (transcribing.value) return t('chat.recognizingAria')
  if (listening.value) return t('chat.stopVoiceAria')
  return t('chat.voiceAria')
})

const newSessionLabel = computed(() => {
  if (sessionRemaining.value == null) return t('chat.newSession')
  return t('chat.newSessionRemaining', { n: sessionRemaining.value })
})

const voiceButtonLabel = computed(() => {
  if (voiceRemaining.value == null) return t('chat.voiceInput')
  return t('chat.voiceRemaining', { n: voiceRemaining.value })
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
  const loc = locale.value === 'en' ? 'en-US' : 'zh-CN'
  return new Date(v).toLocaleString(loc)
}

function sessionLabel(s) {
  const title = s.title || t('chat.untitled')
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

async function onDirectoryChange(value) {
  if (value === '' || value === null) directoryId.value = undefined
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
  Message.success(t('chat.newSessionStarted'))
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

function messageText(m) {
  return m.content === EMPTY_ANSWER ? t('chat.emptyAnswer') : m.content
}

function docOptionLabel(d) {
  const name = d.title || d.file_name || t('chat.docFallback', { id: d.id })
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
    Message.success(t('chat.labelsSaved'))
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
  if (!text || text === EMPTY_ANSWER || text === '(空回答)') return false
  if (asking.value && idx === messages.value.length - 1) return false
  return true
}

function ttsButtonLabel(idx) {
  if (speakingIdx.value === idx) return t('chat.stopSpeak')
  if (ttsRemaining.value === 0) return t('chat.ttsUsedUp')
  return t('chat.speakAnswer')
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
        pushStep('step', evt.message || t('chat.step', { step: evt.step }))
      },
      onToolStart: (evt) => {
        pushStep(
          'tool',
          evt.tool_query ? t('chat.searchingQuery', { query: evt.tool_query }) : t('chat.searching'),
        )
      },
      onToolResult: (evt) => {
        pushStep('result', t('chat.searchDone', { count: evt.tool_count ?? 0 }))
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
          messages.value[assistantIdx].content = EMPTY_ANSWER
        }
        await loadSessions()
        await scrollBottom()
      },
    })
  } catch (err) {
    const msg = err?.message || t('chat.streamFailed')
    if (!messages.value[assistantIdx].content) {
      messages.value[assistantIdx].content = msg
    }
    Message.error(msg)
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

.side :deep(.arco-select-view),
.side :deep(.arco-tree-select) {
  width: 100%;
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

.feedback-rate :deep(.arco-rate) {
  font-size: 16px;
  min-height: 24px;
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

.sources {
  max-width: 780px;
  width: 100%;
}

.sources :deep(.arco-collapse) {
  border: none;
  background: transparent;
}

.sources :deep(.arco-collapse-item-header) {
  background: transparent;
  border: none;
  color: #64748b;
  font-size: 13px;
  padding: 4px 0;
}

.sources :deep(.arco-collapse-item-content) {
  background: transparent;
  padding: 0;
}

.sources :deep(.arco-collapse-item-content-box) {
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
