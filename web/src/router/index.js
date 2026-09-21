import { watch } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import MainLayout from '@/layouts/MainLayout.vue'
import { i18n } from '@/i18n'
import { isLoggedIn, isPlatformAdmin } from '@/utils/auth'

const routes = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/Login.vue'),
    meta: { titleKey: 'auth.loginTitle', public: true },
  },
  {
    path: '/register',
    name: 'register',
    component: () => import('@/views/Register.vue'),
    meta: { titleKey: 'auth.registerTitle', public: true },
  },
  {
    path: '/',
    component: MainLayout,
    redirect: '/knowledge-bases',
    meta: { requiresAuth: true },
    children: [
      {
        path: 'knowledge-bases',
        name: 'knowledge-bases',
        component: () => import('@/views/KnowledgeBases.vue'),
        meta: { titleKey: 'nav.knowledgeBases', requiresAuth: true },
      },
      {
        path: 'knowledge-bases/:id',
        name: 'knowledge-base-detail',
        component: () => import('@/views/KnowledgeBaseDetail.vue'),
        meta: { titleKey: 'kbDetail.fallbackTitle', requiresAuth: true },
      },
      {
        path: 'chat',
        name: 'chat',
        component: () => import('@/views/Chat.vue'),
        meta: { titleKey: 'chat.title', requiresAuth: true },
      },
      {
        path: 'users',
        name: 'users',
        component: () => import('@/views/Users.vue'),
        meta: { titleKey: 'nav.users', requiresAuth: true },
      },
      {
        path: 'tenant-manage',
        name: 'tenant-manage',
        component: () => import('@/views/TenantManage.vue'),
        meta: { titleKey: 'nav.tenantManage', requiresAuth: true, requiresPlatformAdmin: true },
      },
      {
        path: 'tenants',
        name: 'tenants',
        component: () => import('@/views/Tenants.vue'),
        meta: { titleKey: 'nav.createTenant', requiresAuth: true, requiresPlatformAdmin: true },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

router.beforeEach((to) => {
  if (to.meta.public) {
    if (isLoggedIn() && (to.name === 'login' || to.name === 'register')) {
      return { path: '/knowledge-bases' }
    }
    return true
  }
  if (to.meta.requiresAuth && !isLoggedIn()) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.meta.requiresPlatformAdmin && !isPlatformAdmin()) {
    return { path: '/knowledge-bases' }
  }
  return true
})

function syncTitle(to) {
  const key = to.meta?.titleKey
  const name = key ? i18n.global.t(key) : 'Eino RAG'
  document.title = key ? `${name} · Eino RAG` : 'Eino RAG'
}

router.afterEach((to) => {
  syncTitle(to)
})

watch(
  () => i18n.global.locale.value,
  () => syncTitle(router.currentRoute.value),
)

export default router
