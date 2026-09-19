<template>
  <div>
    <div class="page-header">
      <div>
        <h2>{{ t('users.title') }}</h2>
        <p class="sub">{{ t('users.sub') }}</p>
      </div>
    </div>

    <div class="panel">
      <a-table
        row-key="username"
        :columns="columns"
        :data="list"
        :loading="loading"
        :pagination="false"
      >
        <template #created_at="{ record }">
          {{ formatTime(record.created_at) }}
        </template>
        <template #admin="{ record }">
          <a-tag :color="record.is_admin ? 'green' : 'gray'" size="small">
            {{ record.is_admin ? t('common.yes') : t('common.no') }}
          </a-tag>
        </template>
        <template #login="{ record }">
          <a-switch
            :model-value="record.login_enabled"
            :disabled="record.is_admin || toggling === record.username"
            @change="(enabled) => onToggleLogin(record, enabled)"
          />
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
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'
import { getAuthUser } from '@/utils/auth'
import { formatTime } from '@/utils/helpers'

const { t } = useI18n()

const loading = ref(false)
const list = ref([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const toggling = ref('')
const isTenantAdmin = computed(() => getAuthUser()?.username === 'admin')

const columns = computed(() => {
  const cols = [
    { title: t('auth.username'), dataIndex: 'username', ellipsis: true, tooltip: true },
    { title: t('common.createdAt'), slotName: 'created_at', width: 180 },
    { title: t('users.tenantAdmin'), slotName: 'admin', width: 140 },
  ]
  if (isTenantAdmin.value) {
    cols.push({ title: t('users.loginEnabled'), slotName: 'login', width: 120 })
  }
  return cols
})

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

function onPageChange(current) {
  page.value = current
  load()
}

function onPageSizeChange(size) {
  pageSize.value = size
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
    Message.success(enabled ? t('users.loginOn') : t('users.loginOff'))
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
