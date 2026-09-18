<template>
  <el-container class="layout">
    <el-aside width="220px" class="aside">
      <div class="brand">
        <div class="brand-mark">E</div>
        <div>
          <div class="brand-name">Eino RAG</div>
          <div class="brand-sub">知识库管理台</div>
        </div>
      </div>
      <el-menu
        :default-active="active"
        router
        background-color="#0f172a"
        text-color="#cbd5e1"
        active-text-color="#fff"
      >
        <el-menu-item index="/knowledge-bases">
          <el-icon><Collection /></el-icon>
          <span>知识库</span>
        </el-menu-item>
        <el-menu-item index="/chat">
          <el-icon><ChatDotRound /></el-icon>
          <span>知识问答</span>
        </el-menu-item>
        <el-menu-item index="/users">
          <el-icon><User /></el-icon>
          <span>用户管理</span>
        </el-menu-item>
        <el-menu-item v-if="canCreateTenant" index="/tenants">
          <el-icon><OfficeBuilding /></el-icon>
          <span>创建租户</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header" height="56px">
        <div class="header-left">{{ pageTitle }}</div>
        <div class="header-right">
          <el-tag effect="plain" size="small">租户 {{ tenantCode }}</el-tag>
          <span class="user">{{ displayName }}</span>
          <el-tag :type="healthOk ? 'success' : 'danger'" effect="plain" size="small">
            {{ healthOk ? 'API 正常' : 'API 异常' }}
          </el-tag>
          <el-button size="small" @click="logout">退出</el-button>
        </div>
      </el-header>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api'
import { clearAuth, getAuthTenant, getAuthUser, isPlatformAdmin } from '@/utils/auth'

const route = useRoute()
const router = useRouter()
const healthOk = ref(false)
let healthTimer

const user = getAuthUser()
const tenant = getAuthTenant()
const displayName = computed(() => user?.display_name || user?.username || '用户')
const tenantCode = computed(() => tenant?.code || '-')
const canCreateTenant = computed(() => isPlatformAdmin())

const active = computed(() => {
  if (route.path.startsWith('/chat')) return '/chat'
  if (route.path.startsWith('/users')) return '/users'
  if (route.path.startsWith('/tenants')) return '/tenants'
  return '/knowledge-bases'
})

const pageTitle = computed(() => route.meta.title || 'Eino RAG')

function logout() {
  clearAuth()
  router.replace('/login')
}

async function checkHealth() {
  try {
    await api.health()
    healthOk.value = true
  } catch {
    healthOk.value = false
  }
}

onMounted(() => {
  checkHealth()
  healthTimer = setInterval(checkHealth, 30000)
})

onUnmounted(() => {
  if (healthTimer) clearInterval(healthTimer)
})
</script>

<style scoped>
.layout {
  min-height: 100vh;
}

.aside {
  background: var(--app-sidebar);
  color: var(--app-sidebar-text);
  display: flex;
  flex-direction: column;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 20px 16px;
}

.brand-mark {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: linear-gradient(135deg, #3b82f6, #0ea5e9);
  color: #fff;
  display: grid;
  place-items: center;
  font-weight: 700;
}

.brand-name {
  font-weight: 650;
  color: #fff;
}

.brand-sub {
  font-size: 12px;
  color: #94a3b8;
}

.header {
  background: #fff;
  border-bottom: 1px solid #e2e8f0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
}

.header-left {
  font-weight: 600;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.user {
  color: #334155;
  font-size: 13px;
}

.main {
  padding: 20px;
}

:deep(.el-menu) {
  border-right: none;
}
</style>
