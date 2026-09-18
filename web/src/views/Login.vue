<template>
  <div class="auth-page">
    <div class="auth-card">
      <h1>登录</h1>
      <p class="sub">使用租户 ID 与账号登录知识库</p>
      <el-form :model="form" @submit.prevent="onSubmit" label-position="top">
        <el-form-item label="租户 ID" required>
          <el-input v-model="form.tenant_id" placeholder="例如 default" autocomplete="organization" />
        </el-form-item>
        <el-form-item label="用户名" required>
          <el-input v-model="form.username" placeholder="用户名" autocomplete="username" />
        </el-form-item>
        <el-form-item label="密码" required>
          <el-input
            v-model="form.password"
            type="password"
            show-password
            placeholder="密码"
            autocomplete="current-password"
          />
        </el-form-item>
        <el-button type="primary" class="submit" :loading="loading" native-type="submit">
          登录
        </el-button>
      </el-form>
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
import { ElMessage } from 'element-plus'
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
    ElMessage.warning('请填写租户 ID')
    return
  }
  if (!form.username.trim() || !form.password) {
    ElMessage.warning('请填写用户名和密码')
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
    ElMessage.success('登录成功')
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
