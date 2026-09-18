<template>
  <div v-loading="pageLoading">
    <div class="page-header">
      <div>
        <el-button link type="primary" @click="$router.push('/knowledge-bases')">
          ← 返回列表
        </el-button>
        <h2>{{ kb?.name || '知识库详情' }}</h2>
        <p class="sub">
          {{ kb?.description || '管理目录树与文档导入' }}
          <template v-if="kb?.username || kb?.user_id">
            · 创建用户 {{ kb.username || kb.user_id }}
          </template>
        </p>
      </div>
      <div class="actions">
        <el-tooltip
          :disabled="!importQuotaFull"
          content="已达到该租户导入文档数上限"
          placement="top"
        >
          <span>
            <el-button :icon="Upload" type="primary" :disabled="importQuotaFull" @click="openImport">导入文档</el-button>
          </span>
        </el-tooltip>
        <el-button :icon="Refresh" @click="refreshAll">刷新</el-button>
      </div>
    </div>

    <el-row :gutter="16">
      <el-col :span="5">
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

      <el-col :span="19">
        <div class="panel">
          <div class="panel-title">
            <span>
              文档列表
              <el-tag v-if="currentDir" size="small" class="ml8">目录: {{ currentDir.name }}</el-tag>
            </span>
            <div class="doc-toolbar">
              <el-select
                v-model="statusFilter"
                clearable
                placeholder="全部状态"
                style="width: 140px"
                @change="onStatusFilterChange"
              >
                <el-option label="待索引" value="pending" />
                <el-option label="索引中" value="indexing" />
                <el-option label="就绪" value="ready" />
                <el-option label="失败" value="failed" />
              </el-select>
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
            <el-table-column label="上传用户" width="120" show-overflow-tooltip>
              <template #default="{ row }">
                {{ row.username || row.user_id || '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="statusType(row.status)" size="small">
                  {{ statusLabel(row.status) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="分块" width="70">
              <template #default="{ row }">
                <el-button link type="primary" @click="openChunks(row)">{{ row.chunk_count }}</el-button>
              </template>
            </el-table-column>
            <el-table-column label="召回率" width="90">
              <template #default="{ row }">
                <span :title="recallTitle(row)">{{ formatRecall(row.recall) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="引用次数" width="90">
              <template #default="{ row }">
                <span title="历史回答中引用过该文档的次数，同一条回答只计 1 次">{{ row.cited_count ?? 0 }}</span>
              </template>
            </el-table-column>
            <el-table-column label="引用片段" min-width="180" show-overflow-tooltip>
              <template #default="{ row }">
                {{ formatChunkRanks(row.cited_chunks) }}
              </template>
            </el-table-column>
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
            <el-table-column label="操作" width="300" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="showDoc(row)">详情</el-button>
                <el-button
                  link
                  type="primary"
                  :disabled="row.status === 'indexing'"
                  @click="onReindexOne(row)"
                >
                  重新索引
                </el-button>
                <el-button link type="primary" @click="openIndexHistory(row)">索引记录</el-button>
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
            accept=".pdf,.docx,.xlsx,.pptx,.html,.htm,.md,.markdown,.txt,.csv,.json,.png,.jpg,.jpeg,.webp,.tif,.tiff,.bmp,.gif"
            :auto-upload="false"
            :limit="maxUploadFiles"
            :on-change="onFileChange"
            :on-remove="onFileRemove"
            :on-exceed="onFileExceed"
          >
            <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
            <div class="el-upload__text">拖拽或 <em>点击选择</em> 文件</div>
            <template #tip>
              <div class="el-upload__tip">
                支持 PDF（含扫描件 OCR）/ 图片 / DOCX / XLSX / PPTX / HTML / MD / TXT / CSV / JSON（不支持旧版 .doc）
                <br />
                单文件 ≤ {{ maxUploadFileSizeMB }}MB，单次最多 {{ maxUploadFiles }} 个，本租户文件总数上限 {{ maxTenantFiles }}（已用 {{ tenantFileCount }}）
              </div>
            </template>
          </el-upload>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :loading="importing" :disabled="importQuotaFull" @click="doImport">开始导入</el-button>
      </template>
    </el-dialog>

    <!-- 文档详情 -->
    <el-drawer v-model="docDetailVisible" title="文档详情" size="520px">
      <template v-if="docDetail">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="ID">{{ docDetail.id }}</el-descriptions-item>
          <el-descriptions-item label="标题">{{ docDetail.title }}</el-descriptions-item>
          <el-descriptions-item label="文件名">{{ docDetail.file_name }}</el-descriptions-item>
          <el-descriptions-item label="上传用户">{{ docDetail.username || docDetail.user_id || '-' }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="statusType(docDetail.status)" size="small">
              {{ statusLabel(docDetail.status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="分块数">{{ docDetail.chunk_count }}</el-descriptions-item>
          <el-descriptions-item label="引用次数">{{ docDetail.cited_count ?? 0 }}</el-descriptions-item>
          <el-descriptions-item label="引用片段">
            <el-table
              :data="topCitedChunks(docDetail.cited_chunks)"
              size="small"
              empty-text="还没有被引用的片段"
            >
              <el-table-column prop="rank" label="排名" width="70" />
              <el-table-column prop="chunk_index" label="片段标号" width="90" />
              <el-table-column prop="count" label="引用次数" />
            </el-table>
          </el-descriptions-item>
          <el-descriptions-item label="上次索引时间">{{ formatTime(docDetail.last_indexed_at) }}</el-descriptions-item>
          <el-descriptions-item label="MD5">{{ docDetail.content_md5 }}</el-descriptions-item>
          <el-descriptions-item label="大小">{{ formatSize(docDetail.file_size) }}</el-descriptions-item>
          <el-descriptions-item v-if="docDetail.status === 'failed' && docDetail.error_msg" label="错误原因">
            <span class="error-msg">{{ docDetail.error_msg }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </template>
    </el-drawer>

    <el-dialog v-model="chunkVisible" :title="chunkTitle" width="760px">
      <el-table v-loading="chunkLoading" :data="chunkPage" max-height="480" empty-text="还没有分块">
        <el-table-column prop="chunk_index" label="分块标号" width="100" />
        <el-table-column label="分块内容" min-width="480">
          <template #default="{ row }">
            <div class="chunk-content">{{ row.content }}</div>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="chunkRows.length > chunkPageSize" class="pager">
        <el-pagination
          v-model:current-page="chunkPageNo"
          :page-size="chunkPageSize"
          layout="total, prev, pager, next"
          :total="chunkRows.length"
        />
      </div>
    </el-dialog>

    <!-- 索引记录 -->
    <el-drawer v-model="indexHistoryVisible" :title="indexHistoryTitle" size="720px">
      <el-table v-loading="indexHistoryLoading" :data="indexHistory" stripe empty-text="暂无索引记录">
        <el-table-column label="索引触发时间" width="180">
          <template #default="{ row }">{{ formatTime(row.triggered_at) }}</template>
        </el-table-column>
        <el-table-column label="索引结束时间" width="180">
          <template #default="{ row }">{{ formatTime(row.finished_at) }}</template>
        </el-table-column>
        <el-table-column label="本次索引状态" width="120">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)" size="small">
              {{ statusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="错误原因" min-width="160" show-overflow-tooltip>
          <template #default="{ row }">{{ row.error_msg || '-' }}</template>
        </el-table-column>
      </el-table>
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
const pageSize = ref(10)
const total = ref(0)
const selectedIds = ref([])
const statusFilter = ref('')

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
const maxUploadFileSizeMB = ref(50)
const maxUploadFiles = ref(20)
const maxTenantFiles = ref(5)
const tenantFileCount = ref(0)
const maxUploadFileSize = computed(() => maxUploadFileSizeMB.value * 1024 * 1024)
const importQuotaFull = computed(
  () => Number(tenantFileCount.value) >= Number(maxTenantFiles.value) && Number(maxTenantFiles.value) > 0,
)

const docDetailVisible = ref(false)
const docDetail = ref(null)
const chunkVisible = ref(false)
const chunkLoading = ref(false)
const chunkDoc = ref(null)
const chunkRows = ref([])
const chunkPageNo = ref(1)
const chunkPageSize = 10
const chunkTitle = computed(() => {
  const name = chunkDoc.value?.file_name || chunkDoc.value?.title
  return name ? `分块 · ${name}` : '分块'
})
const chunkPage = computed(() => {
  const start = (chunkPageNo.value - 1) * chunkPageSize
  return chunkRows.value.slice(start, start + chunkPageSize)
})
const indexHistoryVisible = ref(false)
const indexHistoryLoading = ref(false)
const indexHistory = ref([])
const indexHistoryDoc = ref(null)
const indexHistoryTitle = computed(() => {
  const name = indexHistoryDoc.value?.file_name || indexHistoryDoc.value?.title
  return name ? `索引记录 · ${name}` : '索引记录'
})

async function loadKb() {
  kb.value = await api.getKnowledgeBase(kbId.value)
}

function formatRecall(recall) {
  if (recall === null || recall === undefined || recall === '') return '—'
  const n = Number(recall)
  if (!Number.isFinite(n)) return '—'
  const pct = n * 100
  const text = Number.isInteger(pct) ? String(pct) : pct.toFixed(1)
  return `${text}%`
}

function recallTitle(row) {
  if (row?.recall === null || row?.recall === undefined) return '还没有相关文档标注'
  const k = row.recall_k ? `Top${row.recall_k} ` : ''
  return `${k}命中 ${row.hit_queries ?? 0} / 标注 ${row.labeled_queries ?? 0}`
}

function topCitedChunks(chunks) {
  if (!Array.isArray(chunks)) return []
  return [...chunks]
    .filter((c) => c && Number(c.count) > 0)
    .sort((a, b) => Number(b.count) - Number(a.count) || Number(a.chunk_index) - Number(b.chunk_index))
    .slice(0, 3)
    .map((c, i) => ({ ...c, rank: i + 1 }))
}

function formatChunkRanks(chunks) {
  const top = topCitedChunks(chunks)
  if (!top.length) return '—'
  return top.map((c) => `#${c.rank} 片段${c.chunk_index} ${c.count}次`).join('；')
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
    if (statusFilter.value) {
      params.status = statusFilter.value
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

function onStatusFilterChange() {
  page.value = 1
  loadDocs()
}

async function refreshAll() {
  pageLoading.value = true
  try {
    await Promise.all([loadKb(), loadTree(), loadDocs(), loadUploadLimits()])
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

function onFileChange(file, files) {
  const raw = file?.raw
  if (raw && raw.size > maxUploadFileSize.value) {
    ElMessage.warning(
      `文件「${file.name}」大小 ${(raw.size / (1024 * 1024)).toFixed(1)}MB 超过限制 ${maxUploadFileSizeMB.value}MB`,
    )
    uploadRef.value?.handleRemove(file)
    fileList.value = files.filter((f) => f.uid !== file.uid)
    return
  }
  fileList.value = files
}

function onFileRemove(_file, files) {
  fileList.value = files
}

function onFileExceed() {
  ElMessage.warning(`单次最多上传 ${maxUploadFiles.value} 个文件`)
}

async function loadUploadLimits() {
  try {
    const data = await api.uploadLimits()
    if (data?.max_upload_file_size_mb > 0) {
      maxUploadFileSizeMB.value = data.max_upload_file_size_mb
    }
    if (data?.max_upload_files > 0) {
      maxUploadFiles.value = data.max_upload_files
    }
    if (data?.max_tenant_files > 0) {
      maxTenantFiles.value = data.max_tenant_files
    }
    tenantFileCount.value = data?.tenant_file_count || 0
  } catch {
    // 使用默认值
  }
}

async function openImport() {
  if (importQuotaFull.value) return
  importForm.title = ''
  importForm.directory_id = currentDir.value?.id
  fileList.value = []
  uploadRef.value?.clearFiles()
  await loadUploadLimits()
  importVisible.value = true
}

async function doImport() {
  if (!fileList.value.length) {
    ElMessage.warning('请选择文件')
    return
  }
  if (fileList.value.length > maxUploadFiles.value) {
    ElMessage.warning(`单次最多上传 ${maxUploadFiles.value} 个文件`)
    return
  }
  for (const f of fileList.value) {
    const size = f.raw?.size || 0
    if (size > maxUploadFileSize.value) {
      ElMessage.warning(
        `文件「${f.name}」大小 ${(size / (1024 * 1024)).toFixed(1)}MB 超过限制 ${maxUploadFileSizeMB.value}MB`,
      )
      return
    }
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
    await Promise.all([loadDocs(), loadUploadLimits()])
  } finally {
    importing.value = false
  }
}

function onSelectionChange(rows) {
  selectedIds.value = rows.map((r) => r.id)
}

async function openIndexHistory(row) {
  indexHistoryDoc.value = row
  indexHistoryVisible.value = true
  indexHistoryLoading.value = true
  indexHistory.value = []
  try {
    const data = await api.listIndexBuilds(row.id)
    indexHistory.value = data?.list || []
  } finally {
    indexHistoryLoading.value = false
  }
}

async function onReindexOne(row) {
  const result = await api.reindexDocuments([row.id])
  const item = result?.items?.[0]
  if (item?.skipped) {
    ElMessage.warning(item.message || '未触发重新索引')
  } else {
    ElMessage.success(item?.message || '已触发重新索引')
  }
  await loadDocs()
  if (indexHistoryVisible.value && indexHistoryDoc.value?.id === row.id) {
    await openIndexHistory(indexHistoryDoc.value)
  }
}

async function showDoc(row) {
  docDetail.value = await api.getDocument(row.id)
  docDetailVisible.value = true
}

async function openChunks(row) {
  chunkDoc.value = row
  chunkRows.value = []
  chunkPageNo.value = 1
  chunkVisible.value = true
  chunkLoading.value = true
  try {
    const list = await api.listDocumentChunks(row.id)
    chunkRows.value = Array.isArray(list) ? list : []
  } catch {
    chunkRows.value = []
  } finally {
    chunkLoading.value = false
  }
}

async function onDeleteOne(row) {
  await ElMessageBox.confirm(`确认删除文档「${row.title}」？将级联清理向量与文件。`, '删除确认', {
    type: 'warning',
  })
  await api.deleteDocument(row.id)
  ElMessage.success('已删除')
  if (indexHistoryDoc.value?.id === row.id) indexHistoryVisible.value = false
  await loadDocs()
}

async function onBatchDelete() {
  await ElMessageBox.confirm(`确认删除选中的 ${selectedIds.value.length} 个文档？`, '批量删除', {
    type: 'warning',
  })
  const deletedIds = selectedIds.value.slice()
  const result = await api.deleteDocuments(deletedIds)
  ElMessage.success(result?.message || '批量删除完成')
  if (deletedIds.includes(indexHistoryDoc.value?.id)) indexHistoryVisible.value = false
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

.doc-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
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

.chunk-content {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
  max-height: 160px;
  overflow: auto;
}
</style>
