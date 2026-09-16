<template>
  <div class="auth-page">
    <div class="auth-card">
      <h1>注册</h1>
      <p class="sub">填写已有租户 ID 创建账号（租户需先由管理员创建）</p>
      <el-form :model="form" @submit.prevent="onSubmit" label-position="top">
        <el-form-item label="租户 ID" required>
          <el-input v-model="form.tenant_id" placeholder="例如 default" autocomplete="organization" />
        </el-form-item>
        <el-form-item label="用户名" required>
          <el-input v-model="form.username" placeholder="用户名" autocomplete="username" />
        </el-form-item>
        <el-form-item label="显示名">
          <el-input v-model="form.display_name" placeholder="可选" />
        </el-form-item>
        <el-form-item label="密码" required>
          <el-input
            v-model="form.password"
            type="password"
            show-password
            placeholder="至少 6 位"
            autocomplete="new-password"
          />
        </el-form-item>
        <el-form-item label="确认密码" required>
          <el-input
            v-model="form.password2"
            type="password"
            show-password
            placeholder="再次输入密码"
            autocomplete="new-password"
          />
        </el-form-item>
        <el-button type="primary" class="submit" :loading="loading" native-type="submit">
          注册并登录
        </el-button>
      </el-form>
      <div class="footer">
        已有账号？
        <router-link to="/login">去登录</router-link>
        <span class="sep">·</span>
        <el-button link type="primary" @click="showTenant = true">创建租户</el-button>
      </div>
    </div>

    <el-dialog v-model="showTenant" title="创建租户" width="400px">
      <el-form :model="tenantForm" label-position="top">
        <el-form-item label="租户 ID" required>
          <el-input v-model="tenantForm.code" placeholder="唯一标识，注册时填写" />
        </el-form-item>
        <el-form-item label="租户名称">
          <el-input v-model="tenantForm.name" placeholder="可选，默认与 ID 相同" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showTenant = false">取消</el-button>
        <el-button type="primary" :loading="tenantLoading" @click="createTenant">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { api } from '@/api'
import { setAuth } from '@/utils/auth'

const router = useRouter()
const loading = ref(false)
const showTenant = ref(false)
const tenantLoading = ref(false)
const form = reactive({
  tenant_id: 'default',
  username: '',
  display_name: '',
  password: '',
  password2: '',
})
const tenantForm = reactive({ code: '', name: '' })

async function createTenant() {
  if (!tenantForm.code.trim()) {
    ElMessage.warning('请填写租户 ID')
    return
  }
  tenantLoading.value = true
  try {
    await api.createTenant({
      code: tenantForm.code.trim(),
      name: tenantForm.name.trim(),
    })
    form.tenant_id = tenantForm.code.trim()
    ElMessage.success('租户已创建，请继续注册')
    showTenant.value = false
  } finally {
    tenantLoading.value = false
  }
}

async function onSubmit() {
  if (!form.tenant_id.trim()) {
    ElMessage.warning('请填写租户 ID')
    return
  }
  if (!form.username.trim()) {
    ElMessage.warning('请填写用户名')
    return
  }
  if (form.password.length < 6) {
    ElMessage.warning('密码至少 6 位')
    return
  }
  if (form.password !== form.password2) {
    ElMessage.warning('两次密码不一致')
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
    ElMessage.success('注册成功')
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

.sep {
  margin: 0 6px;
  color: #cbd5e1;
}
</style>
