<template>
  <div>
    <div class="page-header">
      <div>
        <h2>知识库</h2>
        <p class="sub">管理用户 {{ userId }} 下的知识库容器</p>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate">新建知识库</el-button>
    </div>

    <div class="panel">
      <el-table
        v-loading="loading"
        :data="list"
        stripe
        :default-sort="{ prop: 'id', order: 'descending' }"
      >
        <el-table-column prop="id" label="ID" width="80" sortable />
        <el-table-column prop="name" label="名称" min-width="160">
          <template #default="{ row }">
            <el-button link type="primary" @click="goDetail(row)">{{ row.name }}</el-button>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="220" show-overflow-tooltip />
        <el-table-column
          prop="created_at"
          label="创建时间"
          width="180"
          sortable
          :sort-method="sortByCreatedAt"
        >
          <template #default="{ row }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="goDetail(row)">进入</el-button>
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="onDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑知识库' : '新建知识库'" width="480px">
      <el-form :model="form" label-width="80px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="例如：证券合规库" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="onSave">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { api } from '@/api'
import { getUserId } from '@/utils/helpers'

const router = useRouter()
const userId = getUserId()
const loading = ref(false)
const saving = ref(false)
const list = ref([])
const dialogVisible = ref(false)
const editing = ref(null)
const form = reactive({ name: '', description: '' })

function formatTime(v) {
  if (!v) return '-'
  return new Date(v).toLocaleString()
}

function sortByCreatedAt(a, b) {
  return new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
}

async function load() {
  loading.value = true
  try {
    list.value = (await api.listKnowledgeBases(userId)) || []
  } finally {
    loading.value = false
  }
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
    ElMessage.warning('请填写名称')
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await api.updateKnowledgeBase(editing.value.id, {
        name: form.name,
        description: form.description,
      })
      ElMessage.success('已更新')
    } else {
      await api.createKnowledgeBase({
        user_id: userId,
        name: form.name,
        description: form.description,
      })
      ElMessage.success('已创建')
    }
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function onDelete(row) {
  await ElMessageBox.confirm(`确认删除知识库「${row.name}」？需先清空文档与目录。`, '删除确认', {
    type: 'warning',
  })
  await api.deleteKnowledgeBase(row.id)
  ElMessage.success('已删除')
  await load()
}

function goDetail(row) {
  router.push({ name: 'knowledge-base-detail', params: { id: row.id } })
}

onMounted(load)
</script>
