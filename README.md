# eino-repository-rag

基于 [CloudWeGo Eino](https://github.com/cloudwego/eino) 的企业知识库 RAG 服务。

## 能力

- **用户认证**：租户（`tenants`）+ 用户注册/登录；JWT（HS256）鉴权；注册/登录须填写租户 ID，不存在则拦截提示
- **文档导入**：`POST /api/v1/documents/import`，支持一次导入多个文件；按内容 MD5 在知识库内去重，重复导入直接返回成功且不触发索引；导入记录写入 PostgreSQL，新文件完成后经 **Redis 索引队列**异步构建向量索引（可限流、重试、崩溃回灌）
- **知识库分类目录**：多知识库 + 树形目录；导入归属、列表筛选、检索过滤
- **向量检索**：可配置 `redis` 或 `milvus_lite`（Eino Indexer/Retriever + OpenAI 兼容 Embedding）
- **Hybrid 检索**：稠密向量 + Redis BM25（RRF 融合）；`milvus` 模式自动维护 BM25 sidecar 索引
- **Query 改写 / 多路召回**：LLM 生成 N 条检索改写，与原问题分别召回后再 RRF；重排仍用原问题
- **结构切分**：PDF/PPTX 按页、XLSX 按工作表、Markdown/HTML 按标题；超长段再 Recursive，保留 `page`/`section` meta
- **引用后校验**：生成完成后校验 `[n]` 是否落在 sources 内，清洗幻觉引用
- **Agent / Workflow**：Eino ReAct + `knowledge_retrieve` 多步检索（`POST /api/v1/chat/agent`）；标准线性 RAG 仍走 `/chat/query`
- **Rerank**：OpenAI 兼容 Cross-Encoder 重排（如 SiliconFlow `BAAI/bge-reranker-v2-m3`）
- **Golden Eval**：JSONL 评测集 + Hit@K / Recall@K / MRR（`cmd/eval`）
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
HTTP API (Gin) + JWT
  ├─ 公开：创建租户 / 注册 / 登录 / health
  ├─ 文档导入 → 落盘 + PostgreSQL 记录 → Redis 索引队列 → Index Pipeline
  │                                      ├─ Parser（PDF/Office/文本）
  │                                      ├─ Recursive Splitter
  │                                      ├─ Embedding
  │                                      ├─ Vector Store（redis / milvus_lite）
  │                                      └─ BM25 sidecar（hybrid + milvus 时）
  └─ 问答检索 →（可选）Query 改写多路召回
               → 短期/长期记忆
               → Dense Retriever + BM25（可选 Hybrid/RRF）
               → 跨路 RRF → Rerank（可选）
               → DeepSeek Generate → 回写记忆
  └─ Agent 问答 → ReAct（knowledge_retrieve 多步）→ citation 校验 → 回写记忆
```

索引队列：`kb:index:queue`（等待） / `kb:index:active`（在途，启动回灌） / `kb:index:dlq`（死信）；`rag.index_workers` 控制并发，失败按 `index_max_retries` 重试。
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

> 容器内还支持 `REDIS_PASSWORD`（可置空覆盖本地 yaml）、`JWT_SECRET`、`VECTOR_INDEX_PROVIDER`、`MILVUS_ADDRESS` 等。

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

### 3.2 Query 改写 / 多路召回（可选，默认开启）

```yaml
rag:
  query_expand_enabled: true
  query_expand_n: 2                 # 额外改写条数（不含原问题）
  query_expand_timeout_seconds: 20
```

关闭：`query_expand_enabled: false`。改写失败时自动回退为单路原问题检索。

### 3.3 结构切分 + 引用后校验

```yaml
rag:
  structure_split_enabled: true   # 页/表/标题结构切分
  citation_validate_enabled: true # 清洗越界 [n]
  citation_filter_sources: false  # true 时 done 仅保留被引用的 sources
```

结构切分变更后，需对已有文档执行 **重新索引** 才能生效。流式场景下 `delta` 为原始生成；`done.answer` 与写入记忆的内容为校验后的最终回答。

### 3.4 Golden Eval（检索质量回归）

将 `examples/kb_securities_*.md` 导入知识库并索引完成后，按实际 `knowledge_base_id` / `tenant_id` 编辑 `examples/eval/golden.jsonl`，然后：

```bash
make eval
# 或
go run ./cmd/eval -config configs/config.yaml \
  -golden examples/eval/golden.jsonl -k 5 -out /tmp/eval-report.json
```

输出 Hit@K / Recall@K / MRR。样例相关性可用 `relevant_contains`（内容子串）、`relevant_doc_ids`、`relevant_titles` 任一标注。

### 3.5 Agent 多步检索（可选）

```yaml
agent:
  enabled: true   # false 时不注册 /chat/agent
  max_steps: 4    # 约等于检索轮次上限（内部 MaxRunSteps = max_steps*3）
  tool_top_k: 5
```

与线性 RAG（`/chat/query` 一次检索）不同，Agent 使用 Eino ReAct，由模型按需多次调用 `knowledge_retrieve`（仍走 Hybrid/Expand/Rerank），SSE 会额外推送 `step` / `tool_start` / `tool_result`。延迟与费用更高，适合需要多跳检索的问题。

### 4. 启动服务

本地：

```bash
go run ./cmd/server -config configs/config.yaml
# 或
make run
```

Docker（构建后端 + 前端镜像并与依赖一起启动，默认向量库为 Redis）：

```bash
make docker-up
# 等价：docker compose --profile app up -d --build
# 仅构建：make docker-build
```

- API：http://localhost:8080
- Web：http://localhost:5173（nginx 静态资源；`/api`、`/health` 反代到 `app`）

Milvus 模式：

```bash
VECTOR_INDEX_PROVIDER=milvus_lite docker compose --profile app --profile milvus up -d --build
```

后端镜像使用 `configs/config.docker.yaml`（服务名 `postgres`/`redis`，JSON 日志，`upload_dir=/app/storage/uploads`）。默认监听 `:8080`，已启用 CORS（允许本地前端 `5173` / `3000` 跨域访问）。

### 5. 认证（租户 + JWT）

启动时会自动确保存在租户 `default`。注册/登录都必须填写**已存在的租户 ID**（`tenant_id`），否则返回「租户 ID 不存在」。

```bash
# 可选：创建新租户
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{"code":"acme","name":"Acme 公司"}'

# 注册（租户须已存在）
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","username":"alice","password":"secret1"}'

# 登录
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","username":"alice","password":"secret1"}'
# 响应 data.token → 后续请求头: Authorization: Bearer <token>
```

业务 API（知识库/文档/问答）均需携带 JWT；身份从 token 解析，客户端不可伪造 `user_id`。

### 6. 启动 Web 前端（可选）

前后端分离：后端 API `:8080`，前端 Vite 开发服 `:5173`。

```bash
cd web
npm install
npm run dev
# 或在仓库根目录：make web-install && make web
```

浏览器打开 http://localhost:5173 。开发模式下 Vite 会把 `/api`、`/health` 代理到后端；生产构建通过 `web/.env.production` 的 `VITE_API_BASE` 直连后端（依赖 CORS）。

Docker 前端镜像（`web/Dockerfile`）构建时默认 `VITE_API_BASE=`（同源），由 nginx 反代后端，无需改 `.env.production`。
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

### Agent 多步问答

需 `agent.enabled: true`。请求体与 `/chat/query` 相同：

```bash
curl -N -X POST http://localhost:8080/api/v1/chat/agent \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "s-agent-001",
    "knowledge_base_id": 1,
    "query": "对比合规红线与适当性评估各自关注什么？"
  }'
```

SSE 事件：`meta` → `step` / `tool_start` / `tool_result`（可多轮）→ `delta` → `done`（含 sources）。

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
| `rag` | 切分 / TopK / Hybrid / Query 改写 / 结构切分 / 引用校验 / 索引队列 |
| `rerank` | Cross-Encoder 重排 |
| `memory` | 短期 TTL、消息窗口大小 |
| `agent` | ReAct Agent（enabled / max_steps / tool_top_k） |

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
