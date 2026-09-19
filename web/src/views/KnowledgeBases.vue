<template>
  <div>
    <div class="page-header">
      <div>
        <h2>{{ t('kb.title') }}</h2>
        <p class="sub">{{ t('kb.sub') }}</p>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('kb.create') }}</el-button>
    </div>

    <div class="panel">
      <el-table
        v-loading="loading"
        :data="list"
        stripe
        :default-sort="{ prop: 'id', order: 'descending' }"
      >
        <el-table-column prop="id" label="ID" width="80" sortable />
        <el-table-column prop="name" :label="t('common.name')" min-width="160">
          <template #default="{ row }">
            <el-button link type="primary" @click="goDetail(row)">{{ row.name }}</el-button>
          </template>
        </el-table-column>
        <el-table-column prop="description" :label="t('common.description')" min-width="220" show-overflow-tooltip />
        <el-table-column :label="t('kb.creator')" width="140" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.username || row.user_id || '-' }}
          </template>
        </el-table-column>
        <el-table-column
          prop="created_at"
          :label="t('common.createdAt')"
          width="180"
          sortable
          :sort-method="sortByCreatedAt"
        >
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column :label="t('common.actions')" width="240" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="goDetail(row)">{{ t('kb.enter') }}</el-button>
            <el-button link type="primary" @click="openEdit(row)">{{ t('common.edit') }}</el-button>
            <el-button link type="danger" @click="onDelete(row)">{{ t('common.delete') }}</el-button>
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

    <el-dialog v-model="dialogVisible" :title="editing ? t('kb.editTitle') : t('kb.createTitle')" width="480px">
      <el-form :model="form" label-width="110px">
        <el-form-item :label="t('common.name')" required>
          <el-input v-model="form.name" :placeholder="t('kb.namePh')" />
        </el-form-item>
        <el-form-item :label="t('common.description')">
          <el-input v-model="form.description" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="onSave">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { api } from '@/api'
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

function sortByCreatedAt(a, b) {
  return new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
}

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

function onSizeChange() {
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
    ElMessage.warning(t('kb.needName'))
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await api.updateKnowledgeBase(editing.value.id, {
        name: form.name,
        description: form.description,
      })
      ElMessage.success(t('common.updated'))
    } else {
      await api.createKnowledgeBase({
        name: form.name,
        description: form.description,
      })
      ElMessage.success(t('common.created'))
      page.value = 1
    }
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function onDelete(row) {
  await ElMessageBox.confirm(t('kb.deleteConfirm', { name: row.name }), t('common.confirmDelete'), {
    type: 'warning',
    confirmButtonText: t('common.confirm'),
    cancelButtonText: t('common.cancel'),
  })
  await api.deleteKnowledgeBase(row.id)
  ElMessage.success(t('common.deleted'))
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
</style>
