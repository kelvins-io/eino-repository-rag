<template>
  <div>
    <div class="page-header">
      <div>
        <h2>租户管理</h2>
        <p class="sub">全部租户及其上传配额，仅 default 租户的 admin 可查看和配置</p>
      </div>
    </div>

    <div class="panel">
      <el-table v-loading="loading" :data="list" stripe>
        <el-table-column label="租户" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">
            <div>{{ row.code }}</div>
            <div v-if="row.name && row.name !== row.code" class="tenant-name">{{ row.name }}</div>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column prop="user_count" label="用户数" width="90" />
        <el-table-column label="文件总数" width="150">
          <template #default="{ row }">
            <el-input-number v-model="row.max_files" :min="1" :max="1000000" size="small" controls-position="right" />
          </template>
        </el-table-column>
        <el-table-column label="单文件上限(MB)" width="170">
          <template #default="{ row }">
            <el-input-number v-model="row.max_file_size_mb" :min="1" :max="2048" size="small" controls-position="right" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :loading="saving === row.code" @click="saveLimits(row)">保存</el-button>
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
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '@/api'

const loading = ref(false)
const list = ref([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const saving = ref('')

function formatTime(v) {
  if (!v) return '-'
  return new Date(v).toLocaleString()
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
  if (!Number.isInteger(maxFiles) || maxFiles < 1) {
    ElMessage.warning('文件总数至少为 1')
    return
  }
  if (!Number.isInteger(maxFileSizeMB) || maxFileSizeMB < 1) {
    ElMessage.warning('单文件上限至少为 1MB')
    return
  }
  saving.value = row.code
  try {
    await api.updateTenantLimits({
      code: row.code,
      max_files: maxFiles,
      max_file_size_mb: maxFileSizeMB,
    })
    ElMessage.success('已保存')
  } catch {
    await load()
  } finally {
    saving.value = ''
  }
}

onMounted(load)
</script>

<style scoped>
.tenant-name {
  color: #64748b;
  font-size: 12px;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
