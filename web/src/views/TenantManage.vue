<template>
  <div>
    <div class="page-header">
      <div>
        <h2>租户管理</h2>
        <p class="sub">全部租户及其用户数，仅 default 租户的 admin 可查看</p>
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
        <el-table-column prop="user_count" label="用户数" width="120" />
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
import { api } from '@/api'

const loading = ref(false)
const list = ref([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)

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
