<template>
  <div v-loading="pageLoading">
    <div class="page-header">
      <div>
        <el-button link type="primary" @click="$router.push('/knowledge-bases')">
          ← 返回列表
        </el-button>
        <h2>{{ kb?.name || '知识库详情' }}</h2>
        <p class="sub">{{ kb?.description || '管理目录树与文档导入' }}</p>
      </div>
      <div class="actions">
        <el-button :icon="Upload" type="primary" @click="openImport">导入文档</el-button>
        <el-button :icon="Refresh" @click="refreshAll">刷新</el-button>
      </div>
    </div>

    <el-row :gutter="16">
      <el-col :span="7">
        <div class="panel tree-panel">
          <div class="panel-title">
            <span>目录树</span>
            <el-button size="small" :icon="Plus" @click="openDirCreate(null)">新建根目录</el-button>
          </div>
          <el-tree
            :data="treeData"
            node-key="id"
            highlight-current
            default-expand-all
            :expand-on-click-node="false"
            :props="{ label: 'name', children: 'children' }"
            @node-click="onDirClick"
          >
            <template #default="{ data }">
              <div class="tree-node">
                <span class="tree-label">{{ data.name }}</span>
                <span class="tree-ops" @click.stop>
                  <el-button link size="small" @click="openDirCreate(data)">子目录</el-button>
                  <el-button link size="small" @click="openDirEdit(data)">编辑</el-button>
                  <el-button link size="small" type="danger" @click="onDirDelete(data)">删</el-button>
                </span>
              </div>
            </template>
          </el-tree>
          <el-button class="all-docs" text type="primary" @click="clearDirFilter">
            查看全部文档
          </el-button>
        </div>
      </el-col>

      <el-col :span="17">
        <div class="panel">
          <div class="panel-title">
            <span>
              文档列表
              <el-tag v-if="currentDir" size="small" class="ml8">目录: {{ currentDir.name }}</el-tag>
            </span>
            <div>
              <el-button
                size="small"
                :disabled="!selectedIds.length"
                @click="onBatchReindex"
              >
                重新索引
              </el-button>
              <el-button
                size="small"
                type="danger"
                :disabled="!selectedIds.length"
                @click="onBatchDelete"
              >
                批量删除
              </el-button>
            </div>
          </div>

          <el-table
            v-loading="docLoading"
            :data="docs"
            stripe
            @selection-change="onSelectionChange"
          >
            <el-table-column type="selection" width="48" />
            <el-table-column prop="id" label="ID" width="70" />
            <el-table-column prop="file_name" label="文件名" min-width="140" show-overflow-tooltip />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="statusType(row.status)" size="small">
                  {{ statusLabel(row.status) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="chunk_count" label="分块" width="70" />
            <el-table-column label="上次索引时间" width="170">
              <template #default="{ row }">{{ formatTime(row.last_indexed_at) }}</template>
            </el-table-column>
            <el-table-column prop="file_size" label="大小" width="90">
              <template #default="{ row }">{{ formatSize(row.file_size) }}</template>
            </el-table-column>
            <el-table-column label="更新时间" width="170">
              <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
            </el-table-column>
            <el-table-column label="创建时间" width="170">
              <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="160" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="showDoc(row)">详情</el-button>
                <el-button link type="danger" @click="onDeleteOne(row)">删除</el-button>
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
              @current-change="loadDocs"
              @size-change="onDocSizeChange"
            />
          </div>
        </div>
      </el-col>
    </el-row>

    <!-- 目录弹窗 -->
    <el-dialog v-model="dirVisible" :title="dirEditing ? '编辑目录' : '新建目录'" width="440px">
      <el-form :model="dirForm" label-width="80px">
        <el-form-item label="名称" required>
          <el-input v-model="dirForm.name" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="dirForm.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="dirForm.sort_order" :min="0" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dirVisible = false">取消</el-button>
        <el-button type="primary" :loading="dirSaving" @click="saveDir">保存</el-button>
      </template>
    </el-dialog>

    <!-- 导入弹窗 -->
    <el-dialog v-model="importVisible" title="导入文档" width="520px">
      <el-form label-width="100px">
        <el-form-item label="目标目录">
          <el-tree-select
            v-model="importForm.directory_id"
            :data="treeData"
            clearable
            check-strictly
            node-key="id"
            :props="{ label: 'name', children: 'children', value: 'id' }"
            placeholder="可选，不选则挂到知识库根"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="标题(单文件)">
          <el-input v-model="importForm.title" placeholder="多文件时忽略，默认用文件名" />
        </el-form-item>
        <el-form-item label="文件" required>
          <el-upload
            ref="uploadRef"
            drag
            multiple
            accept=".pdf,.docx,.xlsx,.pptx,.html,.htm,.md,.markdown,.txt,.csv,.json"
            :auto-upload="false"
            :on-change="onFileChange"
            :on-remove="onFileRemove"
          >
            <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
            <div class="el-upload__text">拖拽或 <em>点击选择</em> 文件</div>
            <template #tip>
              <div class="el-upload__tip">支持 PDF / DOCX / XLSX / PPTX / HTML / MD / TXT / CSV / JSON（不支持旧版 .doc）</div>
            </template>
          </el-upload>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :loading="importing" @click="doImport">开始导入</el-button>
      </template>
    </el-dialog>

    <!-- 文档详情 -->
    <el-drawer v-model="docDetailVisible" title="文档详情" size="420px">
      <template v-if="docDetail">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="ID">{{ docDetail.id }}</el-descriptions-item>
          <el-descriptions-item label="标题">{{ docDetail.title }}</el-descriptions-item>
          <el-descriptions-item label="文件名">{{ docDetail.file_name }}</el-descriptions-item>
          <el-descriptions-item label="上传用户">{{ docDetail.user_id || '-' }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="statusType(docDetail.status)" size="small">
              {{ statusLabel(docDetail.status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="分块数">{{ docDetail.chunk_count }}</el-descriptions-item>
          <el-descriptions-item label="上次索引时间">{{ formatTime(docDetail.last_indexed_at) }}</el-descriptions-item>
          <el-descriptions-item label="MD5">{{ docDetail.content_md5 }}</el-descriptions-item>
          <el-descriptions-item label="大小">{{ formatSize(docDetail.file_size) }}</el-descriptions-item>
          <el-descriptions-item v-if="docDetail.status === 'failed' && docDetail.error_msg" label="错误原因">
            <span class="error-msg">{{ docDetail.error_msg }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh, Upload, UploadFilled } from '@element-plus/icons-vue'
import { api } from '@/api'
import {
  formatSize,
  formatTime,
  statusLabel,
  statusType,
} from '@/utils/helpers'

const route = useRoute()
const kbId = computed(() => Number(route.params.id))

const pageLoading = ref(false)
const kb = ref(null)
const treeData = ref([])
const currentDir = ref(null)

const docs = ref([])
const docLoading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const selectedIds = ref([])

const dirVisible = ref(false)
const dirSaving = ref(false)
const dirEditing = ref(null)
const dirParent = ref(null)
const dirForm = reactive({ name: '', description: '', sort_order: 0 })

const importVisible = ref(false)
const importing = ref(false)
const uploadRef = ref()
const fileList = ref([])
const importForm = reactive({ directory_id: undefined, title: '' })

const docDetailVisible = ref(false)
const docDetail = ref(null)

async function loadKb() {
  kb.value = await api.getKnowledgeBase(kbId.value)
}

async function loadTree() {
  treeData.value = (await api.listDirectories(kbId.value)) || []
}

async function loadDocs() {
  docLoading.value = true
  try {
    const params = {
      knowledge_base_id: kbId.value,
      page: page.value,
      page_size: pageSize.value,
    }
    if (currentDir.value?.id) {
      params.directory_id = currentDir.value.id
    }
    const data = await api.listDocuments(params)
    docs.value = data?.list || []
    total.value = data?.total || 0
  } finally {
    docLoading.value = false
  }
}

function onDocSizeChange() {
  page.value = 1
  loadDocs()
}

async function refreshAll() {
  pageLoading.value = true
  try {
    await Promise.all([loadKb(), loadTree(), loadDocs()])
  } finally {
    pageLoading.value = false
  }
}

function onDirClick(data) {
  currentDir.value = data
  page.value = 1
  loadDocs()
}

function clearDirFilter() {
  currentDir.value = null
  page.value = 1
  loadDocs()
}

function openDirCreate(parent) {
  dirEditing.value = null
  dirParent.value = parent
  dirForm.name = ''
  dirForm.description = ''
  dirForm.sort_order = 0
  dirVisible.value = true
}

function openDirEdit(data) {
  dirEditing.value = data
  dirParent.value = null
  dirForm.name = data.name
  dirForm.description = data.description || ''
  dirForm.sort_order = data.sort_order || 0
  dirVisible.value = true
}

async function saveDir() {
  if (!dirForm.name.trim()) {
    ElMessage.warning('请填写目录名称')
    return
  }
  dirSaving.value = true
  try {
    if (dirEditing.value) {
      await api.updateDirectory(dirEditing.value.id, {
        name: dirForm.name,
        description: dirForm.description,
        sort_order: dirForm.sort_order,
      })
      ElMessage.success('目录已更新')
    } else {
      const payload = {
        name: dirForm.name,
        description: dirForm.description,
        sort_order: dirForm.sort_order,
      }
      if (dirParent.value?.id) payload.parent_id = dirParent.value.id
      await api.createDirectory(kbId.value, payload)
      ElMessage.success('目录已创建')
    }
    dirVisible.value = false
    await loadTree()
  } finally {
    dirSaving.value = false
  }
}

async function onDirDelete(data) {
  await ElMessageBox.confirm(`确认删除目录「${data.name}」？`, '删除确认', { type: 'warning' })
  await api.deleteDirectory(data.id)
  ElMessage.success('目录已删除')
  if (currentDir.value?.id === data.id) clearDirFilter()
  await loadTree()
}

function onFileChange(_file, files) {
  fileList.value = files
}

function onFileRemove(_file, files) {
  fileList.value = files
}

function openImport() {
  importForm.title = ''
  importForm.directory_id = currentDir.value?.id
  fileList.value = []
  uploadRef.value?.clearFiles()
  importVisible.value = true
}

async function doImport() {
  if (!fileList.value.length) {
    ElMessage.warning('请选择文件')
    return
  }
  const fd = new FormData()
  fd.append('knowledge_base_id', String(kbId.value))
  if (importForm.directory_id) {
    fd.append('directory_id', String(importForm.directory_id))
  }
  if (importForm.title && fileList.value.length === 1) {
    fd.append('title', importForm.title)
  }
  for (const f of fileList.value) {
    fd.append('files', f.raw)
  }
  importing.value = true
  try {
    const result = await api.importDocuments(fd)
    ElMessage.success(result?.message || `导入完成：新增 ${result?.imported || 0}，重复 ${result?.duplicated || 0}`)
    importVisible.value = false
    importForm.title = ''
    importForm.directory_id = undefined
    fileList.value = []
    uploadRef.value?.clearFiles()
    await loadDocs()
  } finally {
    importing.value = false
  }
}

function onSelectionChange(rows) {
  selectedIds.value = rows.map((r) => r.id)
}

async function showDoc(row) {
  docDetail.value = await api.getDocument(row.id)
  docDetailVisible.value = true
}

async function onDeleteOne(row) {
  await ElMessageBox.confirm(`确认删除文档「${row.title}」？将级联清理向量与文件。`, '删除确认', {
    type: 'warning',
  })
  await api.deleteDocument(row.id)
  ElMessage.success('已删除')
  await loadDocs()
}

async function onBatchDelete() {
  await ElMessageBox.confirm(`确认删除选中的 ${selectedIds.value.length} 个文档？`, '批量删除', {
    type: 'warning',
  })
  const result = await api.deleteDocuments(selectedIds.value)
  ElMessage.success(result?.message || '批量删除完成')
  await loadDocs()
}

async function onBatchReindex() {
  const result = await api.reindexDocuments(selectedIds.value)
  ElMessage.success(result?.message || `已触发 ${result?.triggered || 0} 个文档重新索引`)
  await loadDocs()
}

onMounted(refreshAll)
</script>

<style scoped>
.actions {
  display: flex;
  gap: 8px;
}

.panel-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
  font-weight: 600;
}

.tree-panel {
  min-height: 520px;
}

.tree-node {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-right: 4px;
  gap: 8px;
}

.tree-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tree-ops {
  opacity: 0;
  transition: opacity 0.15s;
}

.tree-node:hover .tree-ops {
  opacity: 1;
}

.all-docs {
  margin-top: 12px;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.ml8 {
  margin-left: 8px;
}

.error-msg {
  color: var(--el-color-danger);
  font-size: 12px;
}
</style>
