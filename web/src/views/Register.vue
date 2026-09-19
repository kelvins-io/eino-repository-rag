<template>
  <div class="auth-page">
    <div class="auth-card">
      <h1>注册</h1>
      <p class="sub">填写已有租户 ID 创建账号（租户需先由管理员创建）</p>
      <a-form :model="form" layout="vertical" @submit="onSubmit">
        <a-form-item field="tenant_id" label="租户 ID" required>
          <a-input
            v-model="form.tenant_id"
            placeholder="请输入已有租户 ID"
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
        <a-form-item field="display_name" label="显示名">
          <a-input v-model="form.display_name" placeholder="可选" />
        </a-form-item>
        <a-form-item field="password" label="密码" required>
          <a-input-password
            v-model="form.password"
            placeholder="至少 6 位"
            :input-attrs="{ autocomplete: 'new-password' }"
          />
        </a-form-item>
        <a-form-item field="password2" label="确认密码" required>
          <a-input-password
            v-model="form.password2"
            placeholder="再次输入密码"
            :input-attrs="{ autocomplete: 'new-password' }"
          />
        </a-form-item>
        <a-button type="primary" class="submit" :loading="loading" html-type="submit">
          注册并登录
        </a-button>
      </a-form>
      <div class="footer">
        已有账号？
        <router-link to="/login">去登录</router-link>
      </div>
    </div>
    <a class="contact" href="mailto:1225807604@qq.com">联系我们：1225807604@qq.com</a>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import { api } from '@/api'
import { setAuth } from '@/utils/auth'

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
    Message.warning('请填写租户 ID')
    return
  }
  if (form.tenant_id.trim().toLowerCase() === 'default') {
    Message.warning('当前租户不允许注册新用户')
    return
  }
  if (!form.username.trim()) {
    Message.warning('请填写用户名')
    return
  }
  if (form.password.length < 6) {
    Message.warning('密码至少 6 位')
    return
  }
  if (form.password !== form.password2) {
    Message.warning('两次密码不一致')
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
    Message.success('注册成功')
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
</style>
