<template>
  <div class="chat-page">
    <div class="page-header">
      <div>
        <h2>知识库问答</h2>
        <p class="sub">基于 RAG 检索 + 会话记忆回答问题</p>
      </div>
      <el-button @click="resetSession">新会话</el-button>
    </div>

    <el-row :gutter="16">
      <el-col :span="6">
        <div class="panel side">
          <el-form label-position="top">
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
              />
            </el-form-item>
            <el-form-item label="Session ID">
              <el-input v-model="sessionId" readonly />
            </el-form-item>
            <el-form-item label="User ID">
              <el-input :model-value="userId" readonly />
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
              <div v-if="m.sources?.length" class="sources">
                <div
                  v-for="(s, i) in m.sources"
                  :key="i"
                  class="source-item"
                >
                  <strong>{{ s.title || s.id || `来源 ${i + 1}` }}</strong>
                  <div>{{ s.content }}</div>
                </div>
              </div>
            </div>
          </div>

          <div class="composer">
            <el-input
              v-model="query"
              type="textarea"
              :rows="3"
              placeholder="输入问题，Enter 发送，Shift+Enter 换行"
              @keydown="onKeydown"
            />
            <el-button
              type="primary"
              :loading="asking"
              :disabled="!kbId || !query.trim()"
              @click="ask"
            >
              发送
            </el-button>
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { nextTick, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '@/api'
import { getSessionId, getUserId, newSessionId } from '@/utils/helpers'

const userId = getUserId()
const sessionId = ref(getSessionId())
const kbs = ref([])
const kbId = ref()
const treeData = ref([])
const directoryId = ref()
const messages = ref([])
const query = ref('')
const asking = ref(false)
const listRef = ref()

async function loadKbs() {
  kbs.value = (await api.listKnowledgeBases(userId)) || []
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
}

async function loadHistory() {
  try {
    const list = await api.chatHistory(sessionId.value)
    messages.value = (list || []).map((m) => ({
      role: m.role,
      content: m.content,
    }))
    await scrollBottom()
  } catch {
    messages.value = []
  }
}

function resetSession() {
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
    ask()
  }
}

async function ask() {
  const q = query.value.trim()
  if (!q || !kbId.value || asking.value) return

  messages.value.push({ role: 'user', content: q })
  query.value = ''
  await scrollBottom()

  asking.value = true
  try {
    const payload = {
      user_id: userId,
      session_id: sessionId.value,
      knowledge_base_id: kbId.value,
      query: q,
    }
    if (directoryId.value) {
      payload.directory_id = directoryId.value
    }
    const resp = await api.chatQuery(payload)
    if (resp?.session_id) sessionId.value = resp.session_id
    messages.value.push({
      role: 'assistant',
      content: resp?.answer || '(空回答)',
      sources: resp?.sources || [],
    })
    await scrollBottom()
  } catch {
    messages.value.push({
      role: 'assistant',
      content: '请求失败，请稍后重试。',
    })
  } finally {
    asking.value = false
  }
}

onMounted(async () => {
  await loadKbs()
  await loadHistory()
})
</script>

<style scoped>
.side {
  min-height: 560px;
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

.sources {
  max-width: 780px;
}

.composer {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  padding: 14px 16px;
  border-top: 1px solid #e2e8f0;
  background: #fff;
}
</style>
