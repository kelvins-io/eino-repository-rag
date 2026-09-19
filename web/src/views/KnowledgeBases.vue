<template>
  <div>
    <div class="page-header">
      <div>
        <h2>{{ t('kb.title') }}</h2>
        <p class="sub">{{ t('kb.sub') }}</p>
      </div>
      <a-button type="primary" @click="openCreate">
        <template #icon><icon-plus /></template>
        {{ t('kb.create') }}
      </a-button>
    </div>

    <div class="panel">
      <a-table
        row-key="id"
        :columns="columns"
        :data="list"
        :loading="loading"
        :pagination="false"
        :scroll="{ x: 1100 }"
      >
        <template #name="{ record }">
          <a-button type="text" size="small" @click="goDetail(record)">{{ record.name }}</a-button>
        </template>
        <template #owner="{ record }">
          <span class="cell-ellipsis">{{ record.username || record.user_id || '-' }}</span>
        </template>
        <template #created_at="{ record }">
          {{ formatTime(record.created_at) }}
        </template>
        <template #ops="{ record }">
          <a-button type="text" size="small" @click="goDetail(record)">{{ t('kb.enter') }}</a-button>
          <a-button type="text" size="small" @click="openEdit(record)">{{ t('common.edit') }}</a-button>
          <a-button type="text" status="danger" size="small" @click="onDelete(record)">{{ t('common.delete') }}</a-button>
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

    <a-modal v-model:visible="dialogVisible" :title="editing ? t('kb.editTitle') : t('kb.createTitle')" :width="480" :footer="false" unmount-on-close>
      <a-form :model="form" auto-label-width>
        <a-form-item field="name" :label="t('common.name')" required>
          <a-input v-model="form.name" :placeholder="t('kb.namePh')" />
        </a-form-item>
        <a-form-item field="description" :label="t('common.description')">
          <a-textarea v-model="form.description" :auto-size="{ minRows: 3, maxRows: 6 }" />
        </a-form-item>
      </a-form>
      <div class="modal-footer">
        <a-button @click="dialogVisible = false">{{ t('common.cancel') }}</a-button>
        <a-button type="primary" :loading="saving" @click="onSave">{{ t('common.save') }}</a-button>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'
import { confirmAction } from '@/utils/ui'
import { formatTime } from '@/utils/helpers'

const { t } = useI18n()

const router = useRouter()
const loading = ref(false)
const saving = ref(false)
const list = ref([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const dialogVisible = ref(false)
const editing = ref(null)
const form = reactive({ name: '', description: '' })

const columns = computed(() => [
  {
    title: 'ID',
    dataIndex: 'id',
    width: 80,
    sortable: {
      sortDirections: ['ascend', 'descend'],
      defaultSortOrder: 'descend',
    },
  },
  { title: t('common.name'), dataIndex: 'name', slotName: 'name', width: 180 },
  { title: t('common.description'), dataIndex: 'description', ellipsis: true, tooltip: true },
  { title: t('kb.creator'), slotName: 'owner', width: 140 },
  {
    title: t('common.createdAt'),
    dataIndex: 'created_at',
    slotName: 'created_at',
    width: 180,
    sortable: {
      sortDirections: ['ascend', 'descend'],
      sorter: (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
    },
  },
  { title: t('common.actions'), slotName: 'ops', width: 220, fixed: 'right' },
])

async function load() {
  loading.value = true
  try {
    const data = await api.listKnowledgeBases({
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

function openCreate() {
  editing.value = null
  form.name = ''
  form.description = ''
  dialogVisible.value = true
}

function openEdit(row) {
  editing.value = row
  form.name = row.name
  form.description = row.description || ''
  dialogVisible.value = true
}

async function onSave() {
  if (!form.name.trim()) {
    Message.warning(t('kb.needName'))
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await api.updateKnowledgeBase(editing.value.id, {
        name: form.name,
        description: form.description,
      })
      Message.success(t('common.updated'))
    } else {
      await api.createKnowledgeBase({
        name: form.name,
        description: form.description,
      })
      Message.success(t('common.created'))
      page.value = 1
    }
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function onDelete(row) {
  try {
    await confirmAction(t('kb.deleteConfirm', { name: row.name }), t('common.confirmDelete'))
  } catch {
    return
  }
  await api.deleteKnowledgeBase(row.id)
  Message.success(t('common.deleted'))
  if (list.value.length <= 1 && page.value > 1) {
    page.value -= 1
  }
  await load()
}

function goDetail(row) {
  router.push({ name: 'knowledge-base-detail', params: { id: row.id } })
}

onMounted(load)
</script>

<style scoped>
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.cell-ellipsis {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
