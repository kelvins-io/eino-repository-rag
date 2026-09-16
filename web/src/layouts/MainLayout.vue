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
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header" height="56px">
        <div class="header-left">{{ pageTitle }}</div>
        <div class="header-right">
          <span class="label">用户 ID</span>
          <el-input
            v-model="userId"
            size="small"
            style="width: 140px"
            @change="onUserChange"
          />
          <el-tag :type="healthOk ? 'success' : 'danger'" effect="plain" size="small">
            {{ healthOk ? 'API 正常' : 'API 异常' }}
          </el-tag>
        </div>
      </el-header>
      <el-main class="main">
        <router-view :key="userId" />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '@/api'
import { getUserId, setUserId } from '@/utils/helpers'

const route = useRoute()
const userId = ref(getUserId())
const healthOk = ref(false)
let healthTimer

const active = computed(() => {
  if (route.path.startsWith('/chat')) return '/chat'
  return '/knowledge-bases'
})

const pageTitle = computed(() => route.meta.title || 'Eino RAG')

function onUserChange(val) {
  setUserId(val)
  userId.value = getUserId()
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

.header-right .label {
  color: #64748b;
  font-size: 13px;
}

.main {
  padding: 20px;
}

:deep(.el-menu) {
  border-right: none;
}
</style>
