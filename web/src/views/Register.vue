<template>
  <div class="auth-page">
    <LanguageSwitch class="auth-lang" />
    <div class="auth-card">
      <h1>{{ t('auth.registerTitle') }}</h1>
      <p class="sub">{{ t('auth.registerSub') }}</p>
      <a-form :model="form" layout="vertical" @submit="onSubmit">
        <a-form-item field="tenant_id" :label="t('auth.tenantId')" required>
          <a-input
            v-model="form.tenant_id"
            :placeholder="t('auth.existingTenantPh')"
            :input-attrs="{ autocomplete: 'organization' }"
          />
        </a-form-item>
        <a-form-item field="username" :label="t('auth.username')" required>
          <a-input
            v-model="form.username"
            :placeholder="t('auth.username')"
            :input-attrs="{ autocomplete: 'username' }"
          />
        </a-form-item>
        <a-form-item field="display_name" :label="t('auth.displayName')">
          <a-input v-model="form.display_name" :placeholder="t('auth.optional')" />
        </a-form-item>
        <a-form-item field="password" :label="t('auth.password')" required>
          <a-input-password
            v-model="form.password"
            :placeholder="t('auth.passwordMinPh')"
            :input-attrs="{ autocomplete: 'new-password' }"
          />
        </a-form-item>
        <a-form-item field="password2" :label="t('auth.confirmPassword')" required>
          <a-input-password
            v-model="form.password2"
            :placeholder="t('auth.confirmPasswordPh')"
            :input-attrs="{ autocomplete: 'new-password' }"
          />
        </a-form-item>
        <a-button type="primary" class="submit" :loading="loading" html-type="submit">
          {{ t('auth.registerAndLogin') }}
        </a-button>
      </a-form>
      <div class="footer">
        {{ t('auth.hasAccount') }}
        <router-link to="/login">{{ t('auth.goLogin') }}</router-link>
      </div>
    </div>
    <a class="contact" href="mailto:1225807604@qq.com">{{ t('auth.contact') }}1225807604@qq.com</a>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Message } from '@arco-design/web-vue'
import LanguageSwitch from '@/components/LanguageSwitch.vue'
import { api } from '@/api'
import { setAuth } from '@/utils/auth'

const { t } = useI18n()

const router = useRouter()
const loading = ref(false)
const form = reactive({
  tenant_id: '',
  username: '',
  display_name: '',
  password: '',
  password2: '',
})

async function onSubmit() {
  if (!form.tenant_id.trim()) {
    Message.warning(t('auth.needTenantId'))
    return
  }
  if (form.tenant_id.trim().toLowerCase() === 'default') {
    Message.warning(t('auth.tenantNoRegister'))
    return
  }
  if (!form.username.trim()) {
    Message.warning(t('auth.needUsername'))
    return
  }
  if (form.password.length < 6) {
    Message.warning(t('auth.passwordMin'))
    return
  }
  if (form.password !== form.password2) {
    Message.warning(t('auth.passwordMismatch'))
    return
  }
  loading.value = true
  try {
    const data = await api.register({
      tenant_id: form.tenant_id.trim(),
      username: form.username.trim(),
      password: form.password,
      display_name: form.display_name.trim(),
    })
    setAuth(data)
    Message.success(t('auth.registerOk'))
    router.replace('/knowledge-bases')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.auth-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  background: #0f172a;
  padding: 24px;
  position: relative;
}

.auth-card {
  width: 100%;
  max-width: 420px;
  background: #fff;
  border-radius: 12px;
  padding: 28px 28px 22px;
}

h1 {
  margin: 0;
  font-size: 22px;
}

.sub {
  margin: 6px 0 20px;
  color: #64748b;
  font-size: 13px;
}

.submit {
  width: 100%;
  margin-top: 4px;
}

.footer {
  margin-top: 16px;
  text-align: center;
  font-size: 13px;
  color: #64748b;
}

.footer a {
  color: #2563eb;
  text-decoration: none;
}

.contact {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 24px;
  text-align: center;
  font-size: 13px;
  color: #94a3b8;
  text-decoration: none;
}

.contact:hover {
  color: #fff;
}

.auth-lang {
  position: absolute;
  top: 20px;
  right: 20px;
}
</style>
