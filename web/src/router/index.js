import { createRouter, createWebHistory } from 'vue-router'
import MainLayout from '@/layouts/MainLayout.vue'
import { isLoggedIn, isPlatformAdmin } from '@/utils/auth'

const routes = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/Login.vue'),
    meta: { title: '登录', public: true },
  },
  {
    path: '/register',
    name: 'register',
    component: () => import('@/views/Register.vue'),
    meta: { title: '注册', public: true },
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
        meta: { title: '知识库', requiresAuth: true },
      },
      {
        path: 'knowledge-bases/:id',
        name: 'knowledge-base-detail',
        component: () => import('@/views/KnowledgeBaseDetail.vue'),
        meta: { title: '知识库详情', requiresAuth: true },
      },
      {
        path: 'chat',
        name: 'chat',
        component: () => import('@/views/Chat.vue'),
        meta: { title: '知识库问答', requiresAuth: true },
      },
      {
        path: 'users',
        name: 'users',
        component: () => import('@/views/Users.vue'),
        meta: { title: '用户管理', requiresAuth: true },
      },
      {
        path: 'tenants',
        name: 'tenants',
        component: () => import('@/views/Tenants.vue'),
        meta: { title: '创建租户', requiresAuth: true, requiresPlatformAdmin: true },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
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

router.afterEach((to) => {
  document.title = to.meta.title
    ? `${to.meta.title} · Eino RAG`
    : 'Eino RAG'
})

export default router
