<template>
  <div>
    <div class="page-header">
      <div>
        <h2>{{ t('tenantManage.title') }}</h2>
        <p class="sub">{{ t('tenantManage.sub') }}</p>
      </div>
    </div>

    <div class="panel">
      <el-table v-loading="loading" :data="list" stripe>
        <el-table-column :label="t('tenantManage.tenant')" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <div>{{ row.code }}</div>
            <div v-if="row.name && row.name !== row.code" class="tenant-name">{{ row.name }}</div>
          </template>
        </el-table-column>
        <el-table-column :label="t('common.createdAt')" width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column prop="user_count" :label="t('tenantManage.userCount')" width="90" />
        <el-table-column :label="t('tenantManage.maxFiles')" width="150">
          <template #default="{ row }">
            <el-input-number v-model="row.max_files" :min="1" :max="1000000" size="small" controls-position="right" />
          </template>
        </el-table-column>
        <el-table-column :label="t('tenantManage.fileCount')" width="120">
          <template #default="{ row }">
            <span :class="{ 'usage-over': overLimit(row.file_count, row.max_files) }">{{ row.file_count ?? 0 }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('tenantManage.maxFileSize')" width="160">
          <template #default="{ row }">
            <el-input-number v-model="row.max_file_size_mb" :min="1" :max="2048" size="small" controls-position="right" />
          </template>
        </el-table-column>
        <el-table-column :label="t('tenantManage.maxSessions')" width="160">
          <template #default="{ row }">
            <el-input-number v-model="row.max_sessions" :min="1" :max="1000000" size="small" controls-position="right" />
          </template>
        </el-table-column>
        <el-table-column :label="t('tenantManage.todaySessions')" width="140">
          <template #default="{ row }">
            <span :class="{ 'usage-over': overLimit(row.today_session_count, row.max_sessions) }">{{ row.today_session_count ?? 0 }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('tenantManage.maxTurns')" width="150">
          <template #default="{ row }">
            <el-input-number v-model="row.max_turns" :min="1" :max="1000000" size="small" controls-position="right" />
          </template>
        </el-table-column>
        <el-table-column :label="t('tenantManage.maxVoice')" width="160">
          <template #default="{ row }">
            <el-input-number v-model="row.max_voice_inputs" :min="1" :max="1000000" size="small" controls-position="right" />
          </template>
        </el-table-column>
        <el-table-column :label="t('tenantManage.todayVoice')" width="170">
          <template #default="{ row }">
            <span :class="{ 'usage-over': overLimit(row.today_voice_inputs, row.max_voice_inputs) }">{{ row.today_voice_inputs ?? 0 }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('tenantManage.maxTts')" width="160">
          <template #default="{ row }">
            <el-input-number v-model="row.max_tts" :min="1" :max="1000000" size="small" controls-position="right" />
          </template>
        </el-table-column>
        <el-table-column :label="t('tenantManage.todayTts')" width="170">
          <template #default="{ row }">
            <span :class="{ 'usage-over': overLimit(row.today_tts_count, row.max_tts) }">{{ row.today_tts_count ?? 0 }}</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('common.actions')" width="90" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :loading="saving === row.code" @click="saveLimits(row)">{{ t('common.save') }}</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          :total="total"
          @current-change="load"
          @size-change="onSizeChange"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { api } from '@/api'
import { formatTime } from '@/utils/helpers'

const { t } = useI18n()

const loading = ref(false)
const list = ref([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const saving = ref('')

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

function onSizeChange() {
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
    ElMessage.warning(t('tenantManage.minFiles'))
    return
  }
  if (!Number.isInteger(maxFileSizeMB) || maxFileSizeMB < 1) {
    ElMessage.warning(t('tenantManage.minFileSize'))
    return
  }
  if (!Number.isInteger(maxSessions) || maxSessions < 1) {
    ElMessage.warning(t('tenantManage.minSessions'))
    return
  }
  if (!Number.isInteger(maxTurns) || maxTurns < 1) {
    ElMessage.warning(t('tenantManage.minTurns'))
    return
  }
  if (!Number.isInteger(maxVoiceInputs) || maxVoiceInputs < 1) {
    ElMessage.warning(t('tenantManage.minVoice'))
    return
  }
  if (!Number.isInteger(maxTTS) || maxTTS < 1) {
    ElMessage.warning(t('tenantManage.minTts'))
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
    ElMessage.success(t('common.saved'))
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
  color: var(--el-color-danger);
  font-weight: 600;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
