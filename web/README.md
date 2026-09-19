# Eino RAG Web

Vue 3 + Arco Design 管理台，对接后端 `/api/v1`。

## 开发

```bash
# 先启动后端 :8080
npm install
npm run dev
```

默认 http://localhost:5173 ，`/api` 与 `/health` 由 Vite 代理到 `http://localhost:8080`。

生产构建使用 `.env.production` 中的 `VITE_API_BASE`，依赖后端 CORS。

## 页面

- `/login`、`/register`：登录 / 注册
- `/knowledge-bases`：知识库列表与 CRUD
- `/knowledge-bases/:id`：目录树、文档导入/管理、重新索引、分块与召回
- `/chat`：知识库问答（会话记忆、标准 RAG / Agent、语音输入与朗读、反馈）
- `/users`：本租户用户管理
- `/tenant-manage`、`/tenants`：平台管理员的租户配额与创建租户
