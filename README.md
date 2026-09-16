# eino-repository-rag

基于 [CloudWeGo Eino](https://github.com/cloudwego/eino) 的企业知识库 RAG 服务。

## 能力

- **文档导入**：`POST /api/v1/documents/import`，支持一次导入多个文件；按内容 MD5 在知识库内去重，重复导入直接返回成功且不触发索引；导入记录写入 PostgreSQL，新文件完成后**自动异步构建向量索引**
- **知识库分类目录**：多知识库 + 树形目录；导入归属、列表筛选、检索过滤
- **向量检索**：可配置 `redis` 或 `milvus_lite`（Eino Indexer/Retriever + OpenAI 兼容 Embedding）
- **Hybrid 检索**：稠密向量 + Redis BM25（RRF 融合）；`milvus` 模式自动维护 BM25 sidecar 索引
- **Rerank**：OpenAI 兼容 Cross-Encoder 重排（如 SiliconFlow `BAAI/bge-reranker-v2-m3`）
- **大模型回答**：DeepSeek（`eino-ext/components/model/deepseek`）
- **记忆机制**
  - 短期记忆：Redis List（会话级，带 TTL）
  - 长期记忆：PostgreSQL `messages` / `conversations` 表
  - 历史压缩：超出条数/token 预算时对较早对话做 LLM 滚动摘要，再按 token 预算裁剪注入 prompt

## 架构

```
客户端
  │
  ▼
HTTP API (Gin)
  ├─ 文档导入 → 落盘 + PostgreSQL 记录 → 异步 Index Pipeline
  │                                      ├─ Recursive Splitter（递归分割）
  │                                      ├─ Embedding
  │                                      ├─ Vector Store（redis / milvus_lite）
  │                                      └─ BM25 sidecar（hybrid + milvus 时）
  └─ 问答检索 → 短期/长期记忆
               → Dense Retriever + BM25（可选 Hybrid/RRF）
               → Rerank（可选）
               → DeepSeek Generate → 回写记忆
```

## 快速开始

### 1. 启动依赖

```bash
# 默认：PostgreSQL + Redis Stack
docker compose up -d

# 若使用 milvus_lite 向量索引，额外启动本地 Milvus Standalone
docker compose --profile milvus up -d
```

### 2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env，填入 DEEPSEEK_API_KEY 与 EMBEDDING_API_KEY
```

> DeepSeek 不提供 Embedding，需配置 OpenAI 兼容 Embedding（OpenAI / SiliconFlow / 通义等）。  
> 维度需与 `embedding.dimensions` 一致；Redis 模式对齐 `redis.vector_dim`，Milvus 模式对齐 `milvus.dimension`。

### 3. 选择向量索引后端

`configs/config.yaml`：

```yaml
vector_index:
  provider: "redis"       # 或 milvus_lite

milvus:
  address: "localhost:19530"
  collection: "kb_docs"
  metric_type: "COSINE"
  dimension: 1024
```

也可用环境变量覆盖：`VECTOR_INDEX_PROVIDER=milvus_lite`、`MILVUS_ADDRESS=localhost:19530`。

> 说明：官方 Milvus Lite（本地 `.db` 文件）目前主要面向 Python；本项目 Go 侧通过 gRPC 连接本地 Milvus Standalone，配置项命名为 `milvus_lite`，便于本地轻量部署。

### 3.1 Hybrid + Rerank（可选，默认开启）

```yaml
rag:
  hybrid_enabled: true   # 稠密向量 + BM25，RRF 融合
  candidate_k: 20        # 每路候选；0 = top_k*4
  rrf_k: 60

rerank:
  enabled: true
  model: "BAAI/bge-reranker-v2-m3"
  base_url: "https://api.siliconflow.cn/v1"
  # api_key 留空则复用 embedding.api_key
  top_n: 5
```

环境变量：`RERANK_API_KEY`、`RERANK_BASE_URL`、`RERANK_MODEL`。

> milvus 模式下首次开启 Hybrid 后，需对已有文档执行一次 **重新索引**，以写入 BM25 sidecar。  
> redis 向量模式直接复用索引上的 `content`/`title` TEXT 字段，无需额外同步。

### 4. 启动服务

```bash
go run ./cmd/server -config configs/config.yaml
```

默认监听 `:8080`，已启用 CORS（允许本地前端 `5173` / `3000` 跨域访问）。

### 5. 启动 Web 前端（可选）

前后端分离：后端 API `:8080`，前端 Vite 开发服 `:5173`。

```bash
cd web
npm install
npm run dev
# 或在仓库根目录：make web-install && make web
```

浏览器打开 http://localhost:5173 。开发模式下 Vite 会把 `/api`、`/health` 代理到后端；生产构建通过 `web/.env.production` 的 `VITE_API_BASE` 直连后端（依赖 CORS）。

前端能力：知识库 CRUD、目录树、文档导入/列表/删除/重新索引、带会话记忆的知识问答。

## API

### 知识库

```bash
# 创建知识库
curl -X POST http://localhost:8080/api/v1/knowledge-bases \
  -H "Content-Type: application/json" \
  -d '{"user_id":"u001","name":"证券合规库","description":"合规与制度文档"}'

# 列表 / 详情 / 更新 / 删除
curl "http://localhost:8080/api/v1/knowledge-bases?user_id=u001"
curl "http://localhost:8080/api/v1/knowledge-bases/1"
curl -X PUT http://localhost:8080/api/v1/knowledge-bases/1 \
  -H "Content-Type: application/json" \
  -d '{"name":"证券合规库","description":"更新说明"}'
curl -X DELETE http://localhost:8080/api/v1/knowledge-bases/1
```

### 分类目录（树形）

```bash
# 在知识库下创建目录（parent_id 可省略表示根目录）
curl -X POST http://localhost:8080/api/v1/knowledge-bases/1/directories \
  -H "Content-Type: application/json" \
  -d '{"name":"合规制度","description":"红线与适当性","sort_order":1}'

curl -X POST http://localhost:8080/api/v1/knowledge-bases/1/directories \
  -H "Content-Type: application/json" \
  -d '{"name":"融资融券","parent_id":1,"sort_order":2}'

# 获取目录树
curl "http://localhost:8080/api/v1/knowledge-bases/1/directories"

# 更新 / 删除目录
curl -X PUT http://localhost:8080/api/v1/directories/1 \
  -H "Content-Type: application/json" \
  -d '{"name":"合规制度","sort_order":1}'
curl -X DELETE http://localhost:8080/api/v1/directories/2
```

### 导入文档

支持一次上传多个文件（表单字段 `file` 或 `files` 均可重复出现）。导入时会计算文件内容 MD5，**同一知识库内**已存在相同 MD5 的文件会返回成功并标记 `duplicated=true`，不重复落盘、不触发索引构建。

```bash
# 单文件
curl -X POST http://localhost:8080/api/v1/documents/import \
  -F "file=@./examples/kb_securities_01_compliance.md" \
  -F "user_id=u001" \
  -F "title=合规红线" \
  -F "knowledge_base_id=1" \
  -F "directory_id=1"

# 多文件
curl -X POST http://localhost:8080/api/v1/documents/import \
  -F "file=@./examples/kb_securities_01_compliance.md" \
  -F "file=@./examples/kb_securities_02_suitability.md" \
  -F "user_id=u001" \
  -F "knowledge_base_id=1" \
  -F "directory_id=1"
```

未传 `knowledge_base_id` 时，会自动归入该用户的「默认知识库」。新文件导入后状态：`pending` → `indexing` → `ready` / `failed`。`title` 仅在单文件导入时生效；多文件时默认使用各自文件名。

### 查询文档

```bash
curl "http://localhost:8080/api/v1/documents?user_id=u001&knowledge_base_id=1&directory_id=1"
curl "http://localhost:8080/api/v1/documents/1"
```

### 删除文档

删除文档会**级联清理**：向量索引中的全部 chunk → 本地上传文件 → PostgreSQL 记录。索引中（`indexing`）的文档会拒绝删除。重新索引前也会先删旧向量，避免残留污染检索。

```bash
# 单文档
curl -X DELETE http://localhost:8080/api/v1/documents/1

# 批量
curl -X POST http://localhost:8080/api/v1/documents/delete \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3]}'
```

### 重新索引

对已导入文档重新触发异步索引构建（适用于索引失败重试，或配置变更后重建）。文档 ID 放在请求 body，支持一次多个；不存在、正在 `indexing`、或请求内重复的 ID 会跳过。

```bash
curl -X POST http://localhost:8080/api/v1/documents/reindex \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3]}'
```

### 知识库问答（带记忆，可按分类过滤）

```bash
curl -X POST http://localhost:8080/api/v1/chat/query \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "u001",
    "session_id": "s-demo-001",
    "knowledge_base_id": 1,
    "directory_id": 1,
    "query": "从业人员有哪些合规红线？"
  }'
```

指定 `directory_id` 时会包含该目录及其子目录文档。同一 `session_id` 会自动带上短期/长期对话上下文。

### 历史记录（长期记忆）

```bash
curl "http://localhost:8080/api/v1/chat/history?session_id=s-demo-001"
```

## 配置说明

见 `configs/config.yaml`：

| 区块 | 说明 |
|------|------|
| `postgres` | 文档导入记录与长期记忆 |
| `redis` | 短期记忆；`provider=redis` 时兼作向量索引 |
| `vector_index` | 向量后端：`redis` / `milvus_lite` |
| `milvus` | `provider=milvus_lite` 时的连接与集合配置 |
| `deepseek` | 对话大模型 |
| `embedding` | OpenAI 兼容向量模型 |
| `rag` | 切分参数 / TopK / 上传目录 |
| `memory` | 短期 TTL、消息窗口大小 |

## 目录结构

```
cmd/server/          # 入口
configs/             # 配置
internal/
  config/            # 配置加载
  model/             # PostgreSQL 模型
  repository/        # 数据访问
  memory/            # 短期(Redis) + 长期(PG) 记忆
  rag/               # Eino RAG 流水线
  service/           # 业务编排
  handler/           # HTTP Handler
  server/            # 路由（含 CORS）
web/                 # Vue3 + Element Plus 前端
storage/uploads/     # 上传文件落盘
examples/            # 示例知识库文档
```

## 技术栈

- Go + Gin（`gin-contrib/cors`）
- CloudWeGo Eino / Eino-Ext（DeepSeek、OpenAI Embedding、Redis/Milvus Indexer/Retriever、Recursive Splitter）
- PostgreSQL + GORM
- Redis Stack（短期记忆；可选向量检索）
- Milvus Standalone（可选，对应 `milvus_lite` 配置）
- Vue 3 + Vite + Element Plus + Vue Router + Axios
