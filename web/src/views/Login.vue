<template>
  <div class="auth-page">
    <div class="auth-card">
      <h1>登录</h1>
      <p class="sub">使用租户 ID 与账号登录知识库</p>
      <a-form :model="form" layout="vertical" @submit="onSubmit">
        <a-form-item field="tenant_id" label="租户 ID" required>
          <a-input
            v-model="form.tenant_id"
            placeholder="例如 default"
            :input-attrs="{ autocomplete: 'organization' }"
          />
        </a-form-item>
        <a-form-item field="username" label="用户名" required>
          <a-input
            v-model="form.username"
            placeholder="用户名"
            :input-attrs="{ autocomplete: 'username' }"
          />
        </a-form-item>
        <a-form-item field="password" label="密码" required>
          <a-input-password
            v-model="form.password"
            placeholder="密码"
            :input-attrs="{ autocomplete: 'current-password' }"
          />
        </a-form-item>
        <a-button type="primary" class="submit" :loading="loading" html-type="submit">
          登录
        </a-button>
      </a-form>
      <div class="footer">
        还没有账号？
        <router-link to="/register">去注册</router-link>
      </div>
    </div>
    <a class="contact" href="mailto:1225807604@qq.com">联系我们：1225807604@qq.com</a>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'
import { setAuth } from '@/utils/auth'

const router = useRouter()
const route = useRoute()
const loading = ref(false)
const form = reactive({
  tenant_id: 'default',
  username: '',
  password: '',
})

async function onSubmit() {
  if (!form.tenant_id.trim()) {
    Message.warning('请填写租户 ID')
    return
  }
  if (!form.username.trim() || !form.password) {
    Message.warning('请填写用户名和密码')
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
    Message.success('登录成功')
    const redirect = route.query.redirect || '/knowledge-bases'
    router.replace(String(redirect))
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
  max-width: 400px;
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
</style>
