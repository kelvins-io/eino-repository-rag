<template>
  <div>
    <div class="page-header">
      <div>
        <h2>{{ t('tenantManage.title') }}</h2>
        <p class="sub">{{ t('tenantManage.sub') }}</p>
      </div>
    </div>

    <div class="panel">
      <a-table
        row-key="code"
        :columns="columns"
        :data="list"
        :loading="loading"
        :pagination="false"
        :scroll="{ x: 2200 }"
      >
        <template #tenant="{ record }">
          <div>{{ record.code }}</div>
          <div v-if="record.name && record.name !== record.code" class="tenant-name">{{ record.name }}</div>
        </template>
        <template #created_at="{ record }">
          {{ formatTime(record.created_at) }}
        </template>
        <template #max_files="{ record }">
          <a-input-number v-model="record.max_files" :min="1" :max="1000000" size="small" :step="1" :precision="0" />
        </template>
        <template #file_count="{ record }">
          <span :class="{ 'usage-over': overLimit(record.file_count, record.max_files) }">{{ record.file_count ?? 0 }}</span>
        </template>
        <template #max_file_size_mb="{ record }">
          <a-input-number v-model="record.max_file_size_mb" :min="1" :max="2048" size="small" :step="1" :precision="0" />
        </template>
        <template #max_sessions="{ record }">
          <a-input-number v-model="record.max_sessions" :min="1" :max="1000000" size="small" :step="1" :precision="0" />
        </template>
        <template #today_session_count="{ record }">
          <span :class="{ 'usage-over': overLimit(record.today_session_count, record.max_sessions) }">{{ record.today_session_count ?? 0 }}</span>
        </template>
        <template #max_turns="{ record }">
          <a-input-number v-model="record.max_turns" :min="1" :max="1000000" size="small" :step="1" :precision="0" />
        </template>
        <template #max_voice_inputs="{ record }">
          <a-input-number v-model="record.max_voice_inputs" :min="1" :max="1000000" size="small" :step="1" :precision="0" />
        </template>
        <template #today_voice_inputs="{ record }">
          <span :class="{ 'usage-over': overLimit(record.today_voice_inputs, record.max_voice_inputs) }">{{ record.today_voice_inputs ?? 0 }}</span>
        </template>
        <template #max_tts="{ record }">
          <a-input-number v-model="record.max_tts" :min="1" :max="1000000" size="small" :step="1" :precision="0" />
        </template>
        <template #today_tts_count="{ record }">
          <span :class="{ 'usage-over': overLimit(record.today_tts_count, record.max_tts) }">{{ record.today_tts_count ?? 0 }}</span>
        </template>
        <template #ops="{ record }">
          <a-button type="text" size="small" :loading="saving === record.code" @click="saveLimits(record)">{{ t('common.save') }}</a-button>
        </template>
      </a-table>

      <div class="pager">
        <a-pagination
          :current="page"
          :page-size="pageSize"
          :total="total"
          show-total
          show-page-size
          :page-size-options="[10, 20, 50, 100]"
          @change="onPageChange"
          @page-size-change="onPageSizeChange"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'
import { formatTime } from '@/utils/helpers'

const { t } = useI18n()

const loading = ref(false)
const list = ref([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const saving = ref('')

const columns = computed(() => [
  { title: t('tenantManage.tenant'), slotName: 'tenant', width: 200 },
  { title: t('common.createdAt'), slotName: 'created_at', width: 180 },
  { title: t('tenantManage.userCount'), dataIndex: 'user_count', width: 90 },
  { title: t('tenantManage.maxFiles'), slotName: 'max_files', width: 150 },
  { title: t('tenantManage.fileCount'), slotName: 'file_count', width: 120 },
  { title: t('tenantManage.maxFileSize'), slotName: 'max_file_size_mb', width: 160 },
  { title: t('tenantManage.maxSessions'), slotName: 'max_sessions', width: 160 },
  { title: t('tenantManage.todaySessions'), slotName: 'today_session_count', width: 140 },
  { title: t('tenantManage.maxTurns'), slotName: 'max_turns', width: 150 },
  { title: t('tenantManage.maxVoice'), slotName: 'max_voice_inputs', width: 160 },
  { title: t('tenantManage.todayVoice'), slotName: 'today_voice_inputs', width: 170 },
  { title: t('tenantManage.maxTts'), slotName: 'max_tts', width: 160 },
  { title: t('tenantManage.todayTts'), slotName: 'today_tts_count', width: 170 },
  { title: t('common.actions'), slotName: 'ops', width: 90, fixed: 'right' },
])

function overLimit(used, limit) {
  const current = Number(used)
  const max = Number(limit)
  if (!Number.isFinite(current) || !Number.isFinite(max)) return false
  return current >= max
}

async function load() {
  loading.value = true
  try {
    const data = await api.listTenants({
      page: page.value,
      page_size: pageSize.value,
    })
    list.value = data?.list || []
    total.value = data?.total || 0
  } finally {
    loading.value = false
  }
}

function onPageChange(current) {
  page.value = current
  load()
}

function onPageSizeChange(size) {
  pageSize.value = size
  page.value = 1
  load()
}

async function saveLimits(row) {
  const maxFiles = Number(row.max_files)
  const maxFileSizeMB = Number(row.max_file_size_mb)
  const maxSessions = Number(row.max_sessions)
  const maxTurns = Number(row.max_turns)
  const maxVoiceInputs = Number(row.max_voice_inputs)
  const maxTTS = Number(row.max_tts)
  if (!Number.isInteger(maxFiles) || maxFiles < 1) {
    Message.warning(t('tenantManage.minFiles'))
    return
  }
  if (!Number.isInteger(maxFileSizeMB) || maxFileSizeMB < 1) {
    Message.warning(t('tenantManage.minFileSize'))
    return
  }
  if (!Number.isInteger(maxSessions) || maxSessions < 1) {
    Message.warning(t('tenantManage.minSessions'))
    return
  }
  if (!Number.isInteger(maxTurns) || maxTurns < 1) {
    Message.warning(t('tenantManage.minTurns'))
    return
  }
  if (!Number.isInteger(maxVoiceInputs) || maxVoiceInputs < 1) {
    Message.warning(t('tenantManage.minVoice'))
    return
  }
  if (!Number.isInteger(maxTTS) || maxTTS < 1) {
    Message.warning(t('tenantManage.minTts'))
    return
  }
  saving.value = row.code
  try {
    await api.updateTenantLimits({
      code: row.code,
      max_files: maxFiles,
      max_file_size_mb: maxFileSizeMB,
      max_sessions: maxSessions,
      max_turns: maxTurns,
      max_voice_inputs: maxVoiceInputs,
      max_tts: maxTTS,
    })
    Message.success(t('common.saved'))
  } catch {
    await load()
  } finally {
    saving.value = ''
  }
}

function reloadIfVisible() {
  if (document.visibilityState === 'visible') load()
}

onMounted(() => {
  load()
  document.addEventListener('visibilitychange', reloadIfVisible)
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', reloadIfVisible)
})
</script>

<style scoped>
.tenant-name {
  color: #64748b;
  font-size: 12px;
}

.usage-over {
  color: var(--app-danger);
  font-weight: 600;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

:deep(.arco-input-number) {
  width: 120px;
}
</style>
