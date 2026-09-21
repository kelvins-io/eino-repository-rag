<template>
  <div class="auth-page">
    <LanguageSwitch class="auth-lang" />
    <div class="auth-card">
      <div class="auth-brand">EINO RAG</div>
      <h1>{{ t('auth.loginTitle') }}</h1>
      <p class="auth-hint">{{ t('auth.loginSub') }}</p>
      <el-form :model="form" @submit.prevent="onSubmit" label-position="top">
        <el-form-item :label="t('auth.tenantId')" required>
          <el-input v-model="form.tenant_id" :placeholder="t('auth.tenantIdExample')" autocomplete="organization" />
        </el-form-item>
        <el-form-item :label="t('auth.username')" required>
          <el-input v-model="form.username" :placeholder="t('auth.username')" autocomplete="username" />
        </el-form-item>
        <el-form-item :label="t('auth.password')" required>
          <el-input
            v-model="form.password"
            type="password"
            show-password
            :placeholder="t('auth.password')"
            autocomplete="current-password"
          />
        </el-form-item>
        <el-button type="primary" class="auth-submit" :loading="loading" native-type="submit">
          {{ t('auth.loginTitle') }}
        </el-button>
      </el-form>
      <div class="auth-footer">
        {{ t('auth.noAccount') }}
        <router-link :to="registerLink">{{ t('auth.goRegister') }}</router-link>
      </div>
    </div>
    <a class="contact" href="mailto:1225807604@qq.com">{{ t('auth.contact') }}1225807604@qq.com</a>
  </div>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import LanguageSwitch from '@/components/LanguageSwitch.vue'
import { api } from '@/api'
import { setAuth } from '@/utils/auth'
import { tenantIdFromQuery } from '@/utils/helpers'

const { t } = useI18n()

const router = useRouter()
const route = useRoute()
const loading = ref(false)
const form = reactive({
  tenant_id: tenantIdFromQuery(route.query) || 'guest',
  username: '',
  password: '',
})

const registerLink = computed(() => {
  const id = form.tenant_id.trim()
  if (!id || id.toLowerCase() === 'default') return '/register'
  return { path: '/register', query: { tenant_id: id } }
})

async function onSubmit() {
  if (!form.tenant_id.trim()) {
    ElMessage.warning(t('auth.needTenantId'))
    return
  }
  if (!form.username.trim() || !form.password) {
    ElMessage.warning(t('auth.needCredentials'))
    return
  }
  loading.value = true
  try {
    const data = await api.login({
      tenant_id: form.tenant_id.trim(),
      username: form.username.trim(),
      password: form.password,
    })
    setAuth(data)
    ElMessage.success(t('auth.loginOk'))
    const redirect = route.query.redirect || '/knowledge-bases'
    router.replace(String(redirect))
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.auth-page {
  --bg: #f4f6fb;
  --panel: #ffffff;
  --line: #e6eaf2;
  --text: #1f2a37;
  --muted: #6b7280;
  --brand: #3b6dff;

  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  position: relative;
  color: var(--text);
  background:
    radial-gradient(1200px 600px at 10% -10%, rgba(59, 109, 255, 0.18), transparent 55%),
    radial-gradient(900px 500px at 100% 0%, rgba(17, 24, 39, 0.08), transparent 50%),
    var(--bg);
}

.auth-card {
  width: 100%;
  max-width: 400px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 16px;
  padding: 28px 28px 24px;
  box-shadow: 0 12px 40px rgba(17, 24, 39, 0.06);
}

.auth-brand {
  color: var(--brand);
  font-weight: 700;
  letter-spacing: 0.04em;
  margin-bottom: 12px;
}

.auth-card h1 {
  margin: 0 0 6px;
  font-size: 24px;
}

.auth-hint {
  margin: 0 0 20px;
  color: var(--muted);
  font-size: 14px;
}

.auth-submit {
  width: 100%;
}

.auth-footer {
  margin-top: 16px;
  text-align: center;
  color: var(--muted);
  font-size: 14px;
}

.auth-footer a {
  color: var(--brand);
  text-decoration: none;
}

.contact {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 24px;
  text-align: center;
  font-size: 13px;
  color: var(--muted);
  text-decoration: none;
}

.contact:hover {
  color: var(--brand);
}

.auth-lang {
  position: absolute;
  top: 20px;
  right: 20px;
}
</style>
