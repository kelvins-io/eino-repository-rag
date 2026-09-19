<template>
  <a-layout class="layout">
    <a-layout-sider class="aside" :width="220">
      <div class="brand">
        <div class="brand-mark">E</div>
        <div>
          <div class="brand-name">Eino RAG</div>
          <div class="brand-sub">知识库管理台</div>
        </div>
      </div>
      <a-menu :selected-keys="[active]" @menu-item-click="onMenu">
        <a-menu-item key="/knowledge-bases">
          <template #icon><icon-bookmark /></template>
          知识库
        </a-menu-item>
        <a-menu-item key="/chat">
          <template #icon><icon-message /></template>
          知识问答
        </a-menu-item>
        <a-menu-item key="/users">
          <template #icon><icon-user /></template>
          用户管理
        </a-menu-item>
        <a-menu-item v-if="canCreateTenant" key="/tenant-manage">
          <template #icon><icon-home /></template>
          租户管理
        </a-menu-item>
        <a-menu-item v-if="canCreateTenant" key="/tenants">
          <template #icon><icon-apps /></template>
          创建租户
        </a-menu-item>
      </a-menu>
    </a-layout-sider>

    <a-layout>
      <a-layout-header class="header">
        <div class="header-left">{{ pageTitle }}</div>
        <div class="header-right">
          <a-tag size="small">租户 {{ tenantCode }}</a-tag>
          <span class="user">{{ displayName }}</span>
          <a-tag :color="healthOk ? 'green' : 'red'" size="small">
            {{ healthOk ? 'API 正常' : 'API 异常' }}
          </a-tag>
          <a-button size="small" @click="logout">退出</a-button>
        </div>
      </a-layout-header>
      <a-layout-content class="main">
        <router-view />
      </a-layout-content>
    </a-layout>
  </a-layout>
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
  if (route.path.startsWith('/tenant-manage')) return '/tenant-manage'
  if (route.path.startsWith('/tenants')) return '/tenants'
  return '/knowledge-bases'
})

const pageTitle = computed(() => route.meta.title || 'Eino RAG')

function onMenu(key) {
  if (!key || key === route.path) return
  router.push(String(key))
}

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
}

.aside :deep(.arco-layout-sider-children) {
  display: flex;
  flex-direction: column;
  background: var(--app-sidebar);
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
  height: 56px;
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
  background: var(--app-bg);
}

:deep(.arco-menu) {
  background: transparent;
}

:deep(.arco-menu-inner) {
  padding: 4px 8px;
}

:deep(.arco-menu-item) {
  background: transparent;
  color: #cbd5e1;
  border-radius: 8px;
}

:deep(.arco-menu-item .arco-icon) {
  color: #cbd5e1;
}

:deep(.arco-menu-item:hover),
:deep(.arco-menu-item.arco-menu-selected) {
  background: rgba(255, 255, 255, 0.08);
  color: #fff;
}

:deep(.arco-menu-item:hover .arco-icon),
:deep(.arco-menu-item.arco-menu-selected .arco-icon) {
  color: #fff;
}
</style>
