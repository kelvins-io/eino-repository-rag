<template>
  <div>
    <div class="page-header">
      <div>
        <h2>创建租户</h2>
        <p class="sub">仅 default 租户的 admin 可新建租户；新建时会自动创建 admin 管理员，初始密码只写入服务端日志</p>
      </div>
    </div>

    <div class="panel form-panel">
      <el-form :model="form" label-width="88px" @submit.prevent="onSubmit">
        <el-form-item label="租户 ID" required>
          <el-input v-model="form.code" placeholder="例如 acme" maxlength="64" />
        </el-form-item>
        <el-form-item label="租户名称">
          <el-input v-model="form.name" placeholder="可选，默认与租户 ID 相同" maxlength="128" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" native-type="submit">创建</el-button>
        </el-form-item>
      </el-form>

      <el-alert
        v-if="created"
        type="success"
        :closable="false"
        show-icon
        :title="`租户 ${created.code} 已创建`"
        :description="`管理员用户名为 ${created.admin_username || 'admin'}。初始密码已打印在服务端日志中，请到运行日志里查找「租户管理员已创建」。`"
      />
    </div>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api } from '@/api'

const loading = ref(false)
const created = ref(null)
const form = reactive({
  code: '',
  name: '',
})

async function onSubmit() {
  const code = form.code.trim()
  if (!code) {
    ElMessage.warning('请填写租户 ID')
    return
  }
  loading.value = true
  try {
    const data = await api.createTenant({
      code,
      name: form.name.trim(),
    })
    created.value = data
    form.code = ''
    form.name = ''
    ElMessage.success('租户已创建，管理员初始密码见服务端日志')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.form-panel {
  max-width: 640px;
}

.form-panel :deep(.el-alert) {
  margin-top: 8px;
}
</style>
