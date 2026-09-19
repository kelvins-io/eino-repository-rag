<template>
  <div>
    <div class="page-header">
      <div>
        <h2>{{ t('tenants.title') }}</h2>
        <p class="sub">{{ t('tenants.sub') }}</p>
      </div>
    </div>

    <div class="panel form-panel">
      <el-form :model="form" label-width="120px" @submit.prevent="onSubmit">
        <el-form-item :label="t('auth.tenantId')" required>
          <el-input v-model="form.code" :placeholder="t('tenants.codePh')" maxlength="64" />
        </el-form-item>
        <el-form-item :label="t('tenants.tenantName')">
          <el-input v-model="form.name" :placeholder="t('tenants.namePh')" maxlength="128" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="loading" native-type="submit">{{ t('common.create') }}</el-button>
        </el-form-item>
      </el-form>

      <el-alert
        v-if="created"
        type="success"
        :closable="false"
        show-icon
        :title="t('tenants.createdTitle', { code: created.code })"
        :description="t('tenants.createdDesc', { username: created.admin_username || 'admin' })"
      />
    </div>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { api } from '@/api'

const { t } = useI18n()

const loading = ref(false)
const created = ref(null)
const form = reactive({
  code: '',
  name: '',
})

async function onSubmit() {
  const code = form.code.trim()
  if (!code) {
    ElMessage.warning(t('auth.needTenantId'))
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
    ElMessage.success(t('tenants.createdMsg'))
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
