<template>
  <div>
    <div class="page-header">
      <div>
        <h2>用户管理</h2>
        <p class="sub">当前租户下的全部用户</p>
      </div>
    </div>

    <div class="panel">
      <el-table v-loading="loading" :data="list" stripe>
        <el-table-column prop="username" label="用户名" min-width="160" show-overflow-tooltip />
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="租户管理员" width="140">
          <template #default="{ row }">
            <el-tag :type="row.is_admin ? 'success' : 'info'" size="small">
              {{ row.is_admin ? '是' : '否' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column v-if="isTenantAdmin" label="允许登录" width="120">
          <template #default="{ row }">
            <el-switch
              :model-value="row.login_enabled"
              :disabled="row.is_admin || toggling === row.username"
              @change="(enabled) => onToggleLogin(row, enabled)"
            />
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
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '@/api'
import { getAuthUser } from '@/utils/auth'

const loading = ref(false)
const list = ref([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const toggling = ref('')
const isTenantAdmin = computed(() => getAuthUser()?.username === 'admin')

function formatTime(v) {
  if (!v) return '-'
  return new Date(v).toLocaleString()
}

async function load() {
  loading.value = true
  try {
    const data = await api.listUsers({
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

async function onToggleLogin(row, enabled) {
  toggling.value = row.username
  try {
    await api.setUserLoginEnabled({
      username: row.username,
      enabled,
    })
    row.login_enabled = enabled
    ElMessage.success(enabled ? '已允许登录' : '已关闭登录')
  } catch {
    await load()
  } finally {
    toggling.value = ''
  }
}

onMounted(load)
</script>

<style scoped>
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
