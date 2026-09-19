<template>
  <div>
    <div class="page-header">
      <div>
        <h2>创建租户</h2>
        <p class="sub">仅 default 租户的 admin 可新建租户；新建时会自动创建 admin 管理员，初始密码只写入服务端日志</p>
      </div>
    </div>

    <div class="panel form-panel">
      <a-form :model="form" auto-label-width @submit="onSubmit">
        <a-form-item field="code" label="租户 ID" required>
          <a-input v-model="form.code" placeholder="例如 acme" :max-length="64" />
        </a-form-item>
        <a-form-item field="name" label="租户名称">
          <a-input v-model="form.name" placeholder="可选，默认与租户 ID 相同" :max-length="128" />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" :loading="loading" html-type="submit">创建</a-button>
        </a-form-item>
      </a-form>

      <a-alert
        v-if="created"
        type="success"
        show-icon
        :title="`租户 ${created.code} 已创建`"
      >
        管理员用户名为 {{ created.admin_username || 'admin' }}。初始密码已打印在服务端日志中，请到运行日志里查找「租户管理员已创建」。
      </a-alert>
    </div>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
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
    Message.warning('请填写租户 ID')
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
    Message.success('租户已创建，管理员初始密码见服务端日志')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.form-panel {
  max-width: 640px;
}

.form-panel :deep(.arco-alert) {
  margin-top: 8px;
}
</style>
