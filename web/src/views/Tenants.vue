<template>
  <div>
    <div class="page-header">
      <div>
        <h2>{{ t('tenants.title') }}</h2>
        <p class="sub">{{ t('tenants.sub') }}</p>
      </div>
    </div>

    <div class="panel form-panel">
      <a-form :model="form" auto-label-width @submit="onSubmit">
        <a-form-item field="code" :label="t('auth.tenantId')" required>
          <a-input v-model="form.code" :placeholder="t('tenants.codePh')" :max-length="64" />
        </a-form-item>
        <a-form-item field="name" :label="t('tenants.tenantName')">
          <a-input v-model="form.name" :placeholder="t('tenants.namePh')" :max-length="128" />
        </a-form-item>
        <a-form-item>
          <a-button type="primary" :loading="loading" html-type="submit">{{ t('common.create') }}</a-button>
        </a-form-item>
      </a-form>

      <a-alert
        v-if="created"
        type="success"
        show-icon
        :title="t('tenants.createdTitle', { code: created.code })"
      >
        {{ t('tenants.createdDesc', { username: created.admin_username || 'admin' }) }}
      </a-alert>
    </div>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Message } from '@arco-design/web-vue'
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
    Message.warning(t('auth.needTenantId'))
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
    Message.success(t('tenants.createdMsg'))
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
