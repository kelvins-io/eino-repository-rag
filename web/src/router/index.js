import { createRouter, createWebHistory } from 'vue-router'
import MainLayout from '@/layouts/MainLayout.vue'

const routes = [
  {
    path: '/',
    component: MainLayout,
    redirect: '/knowledge-bases',
    children: [
      {
        path: 'knowledge-bases',
        name: 'knowledge-bases',
        component: () => import('@/views/KnowledgeBases.vue'),
        meta: { title: '知识库' },
      },
      {
        path: 'knowledge-bases/:id',
        name: 'knowledge-base-detail',
        component: () => import('@/views/KnowledgeBaseDetail.vue'),
        meta: { title: '知识库详情' },
      },
      {
        path: 'chat',
        name: 'chat',
        component: () => import('@/views/Chat.vue'),
        meta: { title: '知识库问答' },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.afterEach((to) => {
  document.title = to.meta.title
    ? `${to.meta.title} · Eino RAG`
    : 'Eino RAG'
})

export default router
