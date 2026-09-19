<template>
  <a-spin :loading="pageLoading" class="page-spin">
    <div class="page-header">
      <div>
        <a-button type="text" @click="$router.push('/knowledge-bases')">← {{ t('kbDetail.back') }}</a-button>
        <h2>{{ kb?.name || t('kbDetail.fallbackTitle') }}</h2>
        <p class="sub">
          {{ kb?.description || t('kbDetail.fallbackDesc') }}
          <template v-if="kb?.username || kb?.user_id">
            · {{ t('kbDetail.creator', { name: kb.username || kb.user_id }) }}
          </template>
        </p>
      </div>
      <div class="actions">
        <a-tooltip :disabled="!importQuotaFull" :content="t('kbDetail.quotaFull')" position="top">
          <span>
            <a-button type="primary" :disabled="importQuotaFull" @click="openImport">
              <template #icon><icon-upload /></template>
              {{ t('kbDetail.import') }}
            </a-button>
          </span>
        </a-tooltip>
        <a-button @click="refreshAll">
          <template #icon><icon-refresh /></template>
          {{ t('kbDetail.refresh') }}
        </a-button>
      </div>
    </div>

    <a-row :gutter="16">
      <a-col :span="5">
        <div class="panel tree-panel">
          <div class="panel-title">
            <span>{{ t('kbDetail.tree') }}</span>
            <a-button size="small" @click="openDirCreate(null)">
              <template #icon><icon-plus /></template>
              {{ t('kbDetail.newRoot') }}
            </a-button>
          </div>
          <a-tree
            block-node
            :data="treeData"
            :field-names="{ key: 'id', title: 'name', children: 'children' }"
            :selected-keys="currentDir ? [currentDir.id] : []"
            v-model:expanded-keys="expandedKeys"
            @select="onTreeSelect"
          >
            <template #extra="node">
              <span class="tree-ops" @click.stop>
                <a-button type="text" size="mini" @click="openDirCreate(node)">{{ t('kbDetail.child') }}</a-button>
                <a-button type="text" size="mini" @click="openDirEdit(node)">{{ t('common.edit') }}</a-button>
                <a-button type="text" status="danger" size="mini" @click="onDirDelete(node)">{{ t('kbDetail.deleteShort') }}</a-button>
              </span>
            </template>
          </a-tree>
          <a-button class="all-docs" type="text" @click="clearDirFilter">{{ t('kbDetail.allDocs') }}</a-button>
        </div>
      </a-col>

      <a-col :span="19">
        <div class="panel">
          <div class="panel-title">
            <span>
              {{ t('kbDetail.docList') }}
              <a-tag v-if="currentDir" size="small" class="ml8">{{ t('kbDetail.dirTag', { name: currentDir.name }) }}</a-tag>
            </span>
            <div class="doc-toolbar">
              <a-select
                v-model="statusFilter"
                allow-clear
                :placeholder="t('kbDetail.allStatus')"
                style="width: 140px"
                @change="onStatusFilterChange"
              >
                <a-option :label="t('status.pending')" value="pending" />
                <a-option :label="t('status.indexing')" value="indexing" />
                <a-option :label="t('status.ready')" value="ready" />
                <a-option :label="t('status.failed')" value="failed" />
              </a-select>
              <a-button size="small" :disabled="!selectedIds.length" @click="onBatchReindex">{{ t('kbDetail.reindex') }}</a-button>
              <a-button size="small" type="primary" status="danger" :disabled="!selectedIds.length" @click="onBatchDelete">
                {{ t('kbDetail.batchDelete') }}
              </a-button>
            </div>
          </div>

          <a-table
            row-key="id"
            :columns="docColumns"
            :data="docs"
            :loading="docLoading"
            :pagination="false"
            :row-selection="rowSelection"
            :scroll="{ x: 1960 }"
            @selection-change="onSelectionChange"
          >
            <template #owner="{ record }">
              <span class="cell-ellipsis">{{ record.username || record.user_id || '-' }}</span>
            </template>
            <template #status="{ record }">
              <a-tag :color="statusType(record.status)" size="small">{{ statusLabel(record.status) }}</a-tag>
            </template>
            <template #chunks="{ record }">
              <a-button type="text" size="small" @click="openChunks(record)">{{ record.chunk_count }}</a-button>
            </template>
            <template #recall="{ record }">
              <span :title="recallTitle(record)">{{ formatRecall(record.recall) }}</span>
            </template>
            <template #cited_count="{ record }">
              <span :title="t('kbDetail.citedHint')">{{ record.cited_count ?? 0 }}</span>
            </template>
            <template #cited_chunks="{ record }">
              <span class="cell-ellipsis" :title="formatChunkRanks(record.cited_chunks)">{{ formatChunkRanks(record.cited_chunks) }}</span>
            </template>
            <template #last_indexed_at="{ record }">{{ formatTime(record.last_indexed_at) }}</template>
            <template #file_size="{ record }">{{ formatSize(record.file_size) }}</template>
            <template #updated_at="{ record }">{{ formatTime(record.updated_at) }}</template>
            <template #created_at="{ record }">{{ formatTime(record.created_at) }}</template>
            <template #ops="{ record }">
              <a-button type="text" size="mini" @click="showDoc(record)">{{ t('kbDetail.detail') }}</a-button>
              <a-tooltip :content="reindexDisabledReason(record)" :disabled="canReindex(record)">
                <span class="inline-action">
                  <a-button type="text" size="mini" :disabled="!canReindex(record)" @click="onReindexOne(record)">{{ t('kbDetail.reindex') }}</a-button>
                </span>
              </a-tooltip>
              <a-button type="text" size="mini" @click="openIndexHistory(record)">{{ t('kbDetail.indexHistory') }}</a-button>
              <a-button type="text" status="danger" size="mini" @click="onDeleteOne(record)">{{ t('common.delete') }}</a-button>
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
      </a-col>
    </a-row>

    <a-modal v-model:visible="dirVisible" :title="dirEditing ? t('kbDetail.editDir') : t('kbDetail.newDir')" :width="440" :footer="false" unmount-on-close>
      <a-form :model="dirForm" auto-label-width>
        <a-form-item field="name" :label="t('common.name')" required>
          <a-input v-model="dirForm.name" />
        </a-form-item>
        <a-form-item field="description" :label="t('common.description')">
          <a-textarea v-model="dirForm.description" :auto-size="{ minRows: 2, maxRows: 4 }" />
        </a-form-item>
        <a-form-item field="sort_order" :label="t('kbDetail.sort')">
          <a-input-number v-model="dirForm.sort_order" :min="0" :step="1" :precision="0" />
        </a-form-item>
      </a-form>
      <div class="modal-footer">
        <a-button @click="dirVisible = false">{{ t('common.cancel') }}</a-button>
        <a-button type="primary" :loading="dirSaving" @click="saveDir">{{ t('common.save') }}</a-button>
      </div>
    </a-modal>

    <a-modal v-model:visible="importVisible" :title="t('kbDetail.importTitle')" :width="560" :footer="false" unmount-on-close>
      <a-form :model="importForm" auto-label-width>
        <a-form-item :label="t('kbDetail.targetDir')">
          <a-tree-select
            v-model="importForm.directory_id"
            :data="treeData"
            allow-clear
            :field-names="{ key: 'id', title: 'name', children: 'children' }"
            :placeholder="t('kbDetail.targetDirPh')"
            :tree-props="{ defaultExpandAll: true }"
          />
        </a-form-item>
        <a-form-item :label="t('kbDetail.titleSingle')">
          <a-input v-model="importForm.title" :placeholder="t('kbDetail.titlePh')" />
        </a-form-item>
        <a-form-item :label="t('kbDetail.file')" required>
          <a-upload
            v-model:file-list="fileList"
            draggable
            multiple
            :auto-upload="false"
            :limit="maxUploadFiles"
            accept=".pdf,.docx,.xlsx,.pptx,.html,.htm,.md,.markdown,.txt,.csv,.json,.png,.jpg,.jpeg,.webp,.tif,.tiff,.bmp,.gif"
            @change="onUploadChange"
            @exceed-limit="onFileExceed"
          />
          <div class="upload-tip">
            {{ t('kbDetail.formats') }}
            <br />
            {{ t('kbDetail.limits', { size: maxUploadFileSizeMB, files: maxUploadFiles, max: maxTenantFiles, used: tenantFileCount }) }}
          </div>
        </a-form-item>
      </a-form>
      <div class="modal-footer">
        <a-button @click="importVisible = false">{{ t('common.cancel') }}</a-button>
        <a-button type="primary" :loading="importing" :disabled="importQuotaFull" @click="doImport">{{ t('kbDetail.startImport') }}</a-button>
      </div>
    </a-modal>

    <a-drawer v-model:visible="docDetailVisible" :title="t('kbDetail.docDetail')" :width="520" :footer="false" unmount-on-close>
      <a-descriptions v-if="docDetail" :column="1" bordered>
        <a-descriptions-item label="ID">{{ docDetail.id }}</a-descriptions-item>
        <a-descriptions-item :label="t('kbDetail.docTitle')">{{ docDetail.title }}</a-descriptions-item>
        <a-descriptions-item :label="t('kbDetail.fileName')">{{ docDetail.file_name }}</a-descriptions-item>
        <a-descriptions-item :label="t('kbDetail.uploader')">{{ docDetail.username || docDetail.user_id || '-' }}</a-descriptions-item>
        <a-descriptions-item :label="t('common.status')">
          <a-tag :color="statusType(docDetail.status)" size="small">{{ statusLabel(docDetail.status) }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item :label="t('kbDetail.chunkCount')">{{ docDetail.chunk_count }}</a-descriptions-item>
        <a-descriptions-item :label="t('kbDetail.citedCount')">{{ docDetail.cited_count ?? 0 }}</a-descriptions-item>
        <a-descriptions-item :label="t('kbDetail.citedChunks')">
          <a-table
            row-key="chunk_index"
            :columns="citedColumns"
            :data="topCitedChunks(docDetail.cited_chunks)"
            :pagination="false"
            size="small"
          >
            <template #empty>{{ t('kbDetail.citedEmpty') }}</template>
          </a-table>
        </a-descriptions-item>
        <a-descriptions-item :label="t('kbDetail.lastIndexed')">{{ formatTime(docDetail.last_indexed_at) }}</a-descriptions-item>
        <a-descriptions-item label="MD5">{{ docDetail.content_md5 }}</a-descriptions-item>
        <a-descriptions-item :label="t('kbDetail.size')">{{ formatSize(docDetail.file_size) }}</a-descriptions-item>
        <a-descriptions-item v-if="docDetail.status === 'failed' && docDetail.error_msg" :label="t('kbDetail.errorReason')">
          <span class="error-msg">{{ docDetail.error_msg }}</span>
        </a-descriptions-item>
      </a-descriptions>
    </a-drawer>

    <a-modal v-model:visible="chunkVisible" :title="chunkTitle" :width="760" :footer="false" unmount-on-close>
      <a-table
        row-key="chunk_index"
        :columns="chunkColumns"
        :data="chunkPage"
        :loading="chunkLoading"
        :pagination="false"
        :scroll="{ y: 480 }"
      >
        <template #content="{ record }">
          <div class="chunk-content">{{ record.content }}</div>
        </template>
        <template #empty>{{ t('kbDetail.noChunks') }}</template>
      </a-table>
      <div v-if="chunkRows.length > chunkPageSize" class="pager">
        <a-pagination
          v-model:current="chunkPageNo"
          :page-size="chunkPageSize"
          :total="chunkRows.length"
          show-total
        />
      </div>
    </a-modal>

    <a-drawer v-model:visible="indexHistoryVisible" :title="indexHistoryTitle" :width="720" :footer="false" unmount-on-close>
      <a-table
        row-key="_key"
        :columns="historyColumns"
        :data="indexHistory"
        :loading="indexHistoryLoading"
        :pagination="false"
      >
        <template #triggered_at="{ record }">{{ formatTime(record.triggered_at) }}</template>
        <template #finished_at="{ record }">{{ formatTime(record.finished_at) }}</template>
        <template #status="{ record }">
          <a-tag :color="statusType(record.status)" size="small">{{ statusLabel(record.status) }}</a-tag>
        </template>
        <template #error_msg="{ record }">
          <span class="cell-ellipsis" :title="record.error_msg || '-'">{{ record.error_msg || '-' }}</span>
        </template>
        <template #empty>{{ t('kbDetail.noIndexHistory') }}</template>
      </a-table>
    </a-drawer>
  </a-spin>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'
import { confirmAction } from '@/utils/ui'
import {
  formatSize,
  formatTime,
  statusLabel,
  statusType,
} from '@/utils/helpers'

const { t } = useI18n()
const route = useRoute()
const kbId = computed(() => Number(route.params.id))

const pageLoading = ref(false)
const kb = ref(null)
const treeData = ref([])
const expandedKeys = ref([])
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
  return name ? t('kbDetail.chunksOf', { name }) : t('kbDetail.chunks')
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
  return name ? t('kbDetail.indexHistoryOf', { name }) : t('kbDetail.indexHistory')
})

const docColumns = computed(() => [
  { title: 'ID', dataIndex: 'id', width: 70 },
  { title: t('kbDetail.fileName'), dataIndex: 'file_name', width: 160, ellipsis: true, tooltip: true },
  { title: t('kbDetail.uploader'), slotName: 'owner', width: 120 },
  { title: t('common.status'), slotName: 'status', width: 110 },
  { title: t('kbDetail.chunks'), slotName: 'chunks', width: 90 },
  { title: t('kbDetail.recall'), slotName: 'recall', width: 90 },
  { title: t('kbDetail.citedCount'), slotName: 'cited_count', width: 100 },
  { title: t('kbDetail.citedChunks'), slotName: 'cited_chunks', width: 180 },
  { title: t('kbDetail.lastIndexed'), slotName: 'last_indexed_at', width: 170 },
  { title: t('kbDetail.size'), slotName: 'file_size', width: 90 },
  { title: t('kbDetail.updatedAt'), slotName: 'updated_at', width: 170 },
  { title: t('common.createdAt'), slotName: 'created_at', width: 170 },
  { title: t('common.actions'), slotName: 'ops', width: 320, fixed: 'right' },
])
const citedColumns = computed(() => [
  { title: t('kbDetail.rank'), dataIndex: 'rank', width: 70 },
  { title: t('kbDetail.chunkNo'), dataIndex: 'chunk_index', width: 90 },
  { title: t('kbDetail.citeCount'), dataIndex: 'count' },
])
const chunkColumns = computed(() => [
  { title: t('kbDetail.chunkIndex'), dataIndex: 'chunk_index', width: 100 },
  { title: t('kbDetail.chunkContent'), slotName: 'content' },
])
const historyColumns = computed(() => [
  { title: t('kbDetail.triggeredAt'), slotName: 'triggered_at', width: 180 },
  { title: t('kbDetail.finishedAt'), slotName: 'finished_at', width: 180 },
  { title: t('kbDetail.buildStatus'), slotName: 'status', width: 120 },
  { title: t('kbDetail.errorReason'), slotName: 'error_msg' },
])
const rowSelection = computed(() => ({
  type: 'checkbox',
  showCheckedAll: true,
  width: 48,
  selectedRowKeys: selectedIds.value,
}))

function collectKeys(nodes, acc = []) {
  for (const n of nodes || []) {
    if (n?.id != null) acc.push(n.id)
    if (n.children?.length) collectKeys(n.children, acc)
  }
  return acc
}

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
  if (row?.recall === null || row?.recall === undefined) return t('kbDetail.recallNone')
  const k = row.recall_k ? `Top${row.recall_k} ` : ''
  return t('kbDetail.recallHit', { k, hit: row.hit_queries ?? 0, labeled: row.labeled_queries ?? 0 })
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
  return top.map((c) => t('kbDetail.chunkRank', { rank: c.rank, index: c.chunk_index, count: c.count })).join(t('kbDetail.chunkRankSep'))
}

async function loadTree() {
  treeData.value = (await api.listDirectories(kbId.value)) || []
  expandedKeys.value = collectKeys(treeData.value)
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
    selectedIds.value = []
  } finally {
    docLoading.value = false
  }
}

function onPageChange(current) {
  page.value = current
  loadDocs()
}

function onPageSizeChange(size) {
  pageSize.value = size
  page.value = 1
  loadDocs()
}

function onStatusFilterChange(value) {
  statusFilter.value = value || ''
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

function onTreeSelect(_keys, data) {
  const node = data?.node
  if (!node || node.id == null) return
  onDirClick(node)
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
    Message.warning(t('kbDetail.needDirName'))
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
      Message.success(t('kbDetail.dirUpdated'))
    } else {
      const payload = {
        name: dirForm.name,
        description: dirForm.description,
        sort_order: dirForm.sort_order,
      }
      if (dirParent.value?.id) payload.parent_id = dirParent.value.id
      await api.createDirectory(kbId.value, payload)
      Message.success(t('kbDetail.dirCreated'))
    }
    dirVisible.value = false
    await loadTree()
  } finally {
    dirSaving.value = false
  }
}

async function onDirDelete(data) {
  try {
    await confirmAction(t('kbDetail.deleteDir', { name: data.name }), t('common.confirmDelete'))
  } catch {
    return
  }
  await api.deleteDirectory(data.id)
  Message.success(t('kbDetail.dirDeleted'))
  if (currentDir.value?.id === data.id) clearDirFilter()
  await loadTree()
}

function onUploadChange(list, fileItem) {
  const raw = fileItem?.file
  if (raw && raw.size > maxUploadFileSize.value) {
    Message.warning(
      t('kbDetail.fileTooLarge', {
        name: fileItem.name,
        size: (raw.size / (1024 * 1024)).toFixed(1),
        limit: maxUploadFileSizeMB.value,
      }),
    )
    fileList.value = list.filter((f) => f.uid !== fileItem.uid)
    return
  }
  fileList.value = list
}

function onFileExceed() {
  Message.warning(t('kbDetail.tooManyFiles', { count: maxUploadFiles.value }))
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
  await loadUploadLimits()
  importVisible.value = true
}

async function doImport() {
  if (!fileList.value.length) {
    Message.warning(t('kbDetail.needFile'))
    return
  }
  if (fileList.value.length > maxUploadFiles.value) {
    Message.warning(t('kbDetail.tooManyFiles', { count: maxUploadFiles.value }))
    return
  }
  for (const f of fileList.value) {
    const size = f.file?.size || 0
    if (!f.file) {
      Message.warning(t('kbDetail.fileReadFailed', { name: f.name || '' }))
      return
    }
    if (size > maxUploadFileSize.value) {
      Message.warning(
        t('kbDetail.fileTooLarge', {
          name: f.name,
          size: (size / (1024 * 1024)).toFixed(1),
          limit: maxUploadFileSizeMB.value,
        }),
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
    fd.append('files', f.file)
  }
  importing.value = true
  try {
    const result = await api.importDocuments(fd)
    Message.success(result?.message || t('kbDetail.importDone', { imported: result?.imported || 0, duplicated: result?.duplicated || 0 }))
    importVisible.value = false
    importForm.title = ''
    importForm.directory_id = undefined
    fileList.value = []
    await Promise.all([loadDocs(), loadUploadLimits()])
  } finally {
    importing.value = false
  }
}

function onSelectionChange(keys) {
  selectedIds.value = keys
}

function canReindex(row) {
  return row?.status !== 'indexing' && row?.source_available !== false
}

function reindexDisabledReason(row) {
  if (row?.status === 'indexing') {
    return t('kbDetail.reindexIndexing')
  }
  if (row?.source_available === false) {
    return t('kbDetail.reindexNoSource')
  }
  return ''
}

async function openIndexHistory(row) {
  indexHistoryDoc.value = row
  indexHistoryVisible.value = true
  indexHistoryLoading.value = true
  indexHistory.value = []
  try {
    const data = await api.listIndexBuilds(row.id)
    indexHistory.value = (data?.list || []).map((item, i) => ({
      ...item,
      _key: item.id != null ? item.id : `${item.triggered_at || 't'}-${i}`,
    }))
  } finally {
    indexHistoryLoading.value = false
  }
}

async function onReindexOne(row) {
  const result = await api.reindexDocuments([row.id])
  const item = result?.items?.[0]
  if (item?.skipped) {
    Message.warning(item.message || t('kbDetail.reindexSkipped'))
  } else {
    Message.success(item?.message || t('kbDetail.reindexStarted'))
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
  try {
    await confirmAction(t('kbDetail.deleteDoc', { title: row.title }), t('common.confirmDelete'))
  } catch {
    return
  }
  await api.deleteDocument(row.id)
  Message.success(t('common.deleted'))
  if (indexHistoryDoc.value?.id === row.id) indexHistoryVisible.value = false
  await loadDocs()
}

async function onBatchDelete() {
  try {
    await confirmAction(t('kbDetail.batchDeleteConfirm', { count: selectedIds.value.length }), t('kbDetail.batchDelete'))
  } catch {
    return
  }
  const deletedIds = selectedIds.value.slice()
  const result = await api.deleteDocuments(deletedIds)
  Message.success(result?.message || t('kbDetail.batchDeleteDone'))
  if (deletedIds.includes(indexHistoryDoc.value?.id)) indexHistoryVisible.value = false
  await loadDocs()
}

async function onBatchReindex() {
  const result = await api.reindexDocuments(selectedIds.value)
  Message.success(result?.message || t('kbDetail.batchReindexDone', { count: result?.triggered || 0 }))
  await loadDocs()
}

onMounted(refreshAll)
</script>

<style scoped>
.inline-action {
  display: inline-flex;
  vertical-align: middle;
}

.page-spin {
  display: block;
  width: 100%;
}

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

.tree-ops {
  display: inline-flex;
  gap: 2px;
  opacity: 0;
}

:deep(.arco-tree-node:hover) .tree-ops {
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
  color: var(--app-danger);
  font-size: 12px;
}

.chunk-content {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
  max-height: 160px;
  overflow: auto;
}

.cell-ellipsis {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.upload-tip {
  margin-top: 8px;
  color: #86909c;
  font-size: 12px;
  line-height: 1.5;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}
</style>
