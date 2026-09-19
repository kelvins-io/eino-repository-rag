# eino-repository-rag

基于 [CloudWeGo Eino](https://github.com/cloudwego/eino) 的企业知识库 RAG 服务：多租户 JWT 鉴权、文档导入与异步索引、Hybrid 检索、流式问答，以及 Vue 3 + Arco Design 管理台。

## 能力

- **多租户认证**：租户（`tenants`）+ 用户注册/登录；JWT（HS256）；身份从 token 解析，客户端不可伪造 `user_id`
  - 启动时自动确保租户 `default` 及其 `admin`（初始密码只写一次日志）
  - `default` 租户禁止自助注册；其他租户可注册
  - 平台管理员（`default` / `admin`）可创建租户、查看配额、调整限额
  - 各租户 `admin` 可开关本租户用户登录
- **租户配额**：文件总数 / 单文件大小 / 每日新建会话 / 单会话轮次 / 每日语音输入 / 每日 TTS（默认均为 5）
- **文档导入**：`POST /api/v1/documents/import`，一次多个文件；知识库内按内容 MD5 去重；经 **Redis 索引队列**异步构建向量索引（限流、重试、崩溃回灌）
- **文档解析 / OCR**：PDF、Office（DOCX/XLSX/PPTX）、Markdown/HTML/文本/CSV/JSON、图片；扫描 PDF / 图片 / 无文字 PPTX·DOCX 回退 Tesseract（compose `ocr` 服务，本机无需 brew）
- **知识库分类目录**：多知识库 + 树形目录；导入归属、列表筛选、检索过滤（含子目录）；租户内知识库共享可读
- **向量检索**：可配置 `redis` 或 `milvus_lite`（Eino Indexer/Retriever + OpenAI 兼容 Embedding）
- **Hybrid 检索**：稠密向量 + Redis BM25（RRF 融合）；`milvus` 模式自动维护 BM25 sidecar
- **Query 改写 / 多路召回**：LLM 生成 N 条改写，与原问题分别召回后再 RRF；重排仍用原问题
- **结构切分**：PDF/PPTX 按页、XLSX 按工作表、Markdown/HTML 按标题；超长段再 Recursive，保留 `page`/`section` meta
- **引用后校验**：生成完成后校验 `[n]` 是否落在 sources 内，清洗幻觉引用
- **Agent / Workflow**：Eino ReAct + `knowledge_retrieve` 多步检索（`POST /api/v1/chat/agent`）；标准线性 RAG 走 `/chat/query`（均为 SSE）
- **Rerank**：OpenAI 兼容 Cross-Encoder（如 SiliconFlow `BAAI/bge-reranker-v2-m3`）
- **语音**：ASR 语音输入（SenseVoice / OpenAI 兼容 transcriptions）+ TTS 回答朗读（CosyVoice / OpenAI 兼容 speech）
- **问答反馈**：对助手回答点赞/点踩/1–5 分；为用户问题标注相关文档，用于文档召回率
- **检索可观测**：文档分块列表、索引构建历史、文档召回率（Hit@K）与引用次数
- **Golden Eval**：JSONL 评测集 + Hit@K / Recall@K / MRR（`cmd/eval`）
- **大模型回答**：DeepSeek（`eino-ext/components/model/deepseek`）
- **记忆机制**
  - 短期记忆：Redis List（会话级，带 TTL）
  - 长期记忆：PostgreSQL `messages` / `conversations`
  - 历史压缩：超出条数/token 预算时对较早对话做 LLM 滚动摘要，再按 token 预算裁剪注入 prompt

## 架构

```
客户端
  │
  ▼
HTTP API (Gin) + JWT
  ├─ 公开：注册 / 登录 / health
  ├─ 管理（需登录）
  │    ├─ 平台管理员：创建租户 / 租户列表 / 配额
  │    └─ 租户管理员：用户列表 / 开关登录
  ├─ 文档导入 → 落盘 + PostgreSQL 记录 → Redis 索引队列 → Index Pipeline
  │                                      ├─ Parser（PDF/Office/文本/图片 OCR）
  │                                      ├─ Recursive / 结构切分
  │                                      ├─ Embedding
  │                                      ├─ Vector Store（redis / milvus_lite）
  │                                      └─ BM25 sidecar（hybrid + milvus 时）
  └─ 问答检索 →（可选）Query 改写多路召回
               → 短期/长期记忆
               → Dense Retriever + BM25（可选 Hybrid/RRF）
               → 跨路 RRF → Rerank（可选）
               → DeepSeek Generate → citation 校验 → 回写记忆
  └─ Agent 问答 → ReAct（knowledge_retrieve 多步）→ citation 校验 → 回写记忆
  └─ 语音 → ASR 转写提问 / TTS 朗读回答（计入租户日配额）
```

索引队列：`kb:index:queue`（等待） / `kb:index:active`（在途，启动回灌） / `kb:index:dlq`（死信）；`rag.index_workers` 控制并发，失败按 `index_max_retries` 重试。

## 快速开始

### 1. 启动依赖

```bash
# 默认：PostgreSQL + Redis Stack + OCR sidecar
docker compose up -d

# 若使用 milvus_lite 向量索引，额外启动本地 Milvus Standalone
docker compose --profile milvus up -d
```

PostgreSQL 映射 `15433`，Redis 映射 `16379`，OCR 映射 `18080`。本地 `configs/config.yaml` 默认 `vector_index.provider: milvus_lite`，Docker 应用镜像默认 `redis`。

### 2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env，填入 DEEPSEEK_API_KEY 与 EMBEDDING_API_KEY
```

> 容器内还支持 `REDIS_PASSWORD`（可置空覆盖本地 yaml）、`JWT_SECRET`、`VECTOR_INDEX_PROVIDER`、`MILVUS_ADDRESS` 等。

> DeepSeek 不提供 Embedding，需配置 OpenAI 兼容 Embedding（OpenAI / SiliconFlow / 通义等）。
> 维度需与向量库一致：Redis 对齐 `redis.vector_dim`，Milvus 对齐 `milvus.dimension`（`BAAI/bge-m3` 为 1024）。对 bge-m3 不要传 `embedding.dimensions`（SiliconFlow 会报 20015）。

### 3. 选择向量索引后端

`configs/config.yaml`：

```yaml
vector_index:
  provider: "milvus_lite"   # 或 redis

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

### 3.6 扫描件 / 图片 OCR

解析器先抽 PDF 内嵌文本；**图片页、扫描件、乱码 CJK PDF** 以及 **PNG/JPG 等图片** 回退 Tesseract。无文字的 PPTX 幻灯片 / DOCX 会识别 `media` 里的图片。

OCR 跑在 compose 的 `ocr` 服务里（镜像内含 tesseract + 中文语言包 + poppler），本机 **不必** `brew install`。

```yaml
rag:
  ocr:
    enabled: true
    endpoint: "http://localhost:18080"  # 容器内为 http://ocr:8080
    languages: "chi_sim+eng"
    dpi: 200
    concurrency: 1
    timeout_seconds: 60
    page_seg_mode: 6
  index_job_timeout_minutes: 30
```

```bash
docker compose up -d ocr
# 或随依赖一起：docker compose up -d
```

环境变量 `OCR_ENDPOINT` 可覆盖 endpoint。留空 endpoint 时回退本机 `tesseract`/`pdftoppm`。关闭：`rag.ocr.enabled: false`。

### 3.7 语音输入 / 回答朗读（可选，默认开启）

```yaml
asr:
  enabled: true
  model: "FunAudioLLM/SenseVoiceSmall"
  base_url: "https://api.siliconflow.cn/v1"
  language: "zh"
  max_audio_mb: 8
  # api_key 留空则复用 embedding.api_key

tts:
  enabled: true
  model: "FunAudioLLM/CosyVoice2-0.5B"
  voice: "anna"          # 可换 bella、alex 等
  max_chars: 4000
  # api_key 留空则复用 asr / embedding
```

环境变量：`ASR_API_KEY`、`ASR_BASE_URL`、`ASR_MODEL`、`TTS_API_KEY`、`TTS_BASE_URL`、`TTS_MODEL`、`TTS_VOICE`。关闭：`asr.enabled: false` / `tts.enabled: false`。

### 4. 启动服务

本地（需 Go 1.26+）：

```bash
go run ./cmd/server -config configs/config.yaml
# 或
make run
```

首次启动会在日志中打印 `default` 租户 `admin` 的初始密码（仅创建时一次）。生产请用 `JWT_SECRET` 覆盖默认密钥。

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

后端镜像使用 `configs/config.docker.yaml`（服务名 `postgres`/`redis`，JSON 日志，`upload_dir=/app/storage/uploads`）。默认监听 `:8080`，已启用 CORS。

### 5. 认证（租户 + JWT）

启动时会自动确保存在租户 `default` 及其管理员 `admin`。业务 API 均需 `Authorization: Bearer <token>`。

```bash
# 使用日志中的 default/admin 密码登录
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","username":"admin","password":"<日志中的初始密码>"}'
# 响应 data.token → 后续请求头: Authorization: Bearer <token>

# 当前用户
curl http://localhost:8080/api/v1/auth/me \
  -H "Authorization: Bearer <token>"
```

**平台管理员**（`default` 租户的 `admin`）可创建租户。新租户会自动创建 `admin`，密码同样只写服务日志，响应里只有 `admin_username`。

```bash
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"code":"acme","name":"Acme 公司"}'
```

其他租户可自助注册（`default` 不允许）：

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"acme","username":"alice","password":"secret1"}'
```

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

前端能力：登录/注册、知识库 CRUD、目录树、文档导入/列表/删除/重新索引/分块与召回、带会话记忆的知识问答（标准 RAG / Agent）、语音输入与朗读、回答反馈、用户管理；平台管理员另有租户管理与配额配置。

## API

除特别注明外，均需 `Authorization: Bearer <token>`。`user_id` / `tenant_id` 由 JWT 注入，请求体不必也不应伪造。

### 认证与管理

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/auth/register` | 注册（公开；`default` 租户拒绝） |
| POST | `/api/v1/auth/login` | 登录（公开） |
| GET | `/api/v1/auth/me` | 当前用户 |
| GET | `/api/v1/users` | 本租户用户列表 |
| PUT | `/api/v1/users/login-enabled` | 租户 admin 开关登录 |
| POST | `/api/v1/tenants` | 平台管理员创建租户 |
| GET | `/api/v1/tenants` | 平台管理员租户列表与用量 |
| PUT | `/api/v1/tenants/limits` | 平台管理员调整配额 |
| GET | `/health` | 健康检查（公开） |

```bash
# 租户 admin 禁止某用户登录
curl -X PUT http://localhost:8080/api/v1/users/login-enabled \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","enabled":false}'

# 调整租户配额
curl -X PUT http://localhost:8080/api/v1/tenants/limits \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"code":"acme","max_files":20,"max_file_size_mb":50,"max_sessions":20,"max_turns":30,"max_voice_inputs":20,"max_tts":20}'
```

### 知识库

```bash
# 创建知识库
curl -X POST http://localhost:8080/api/v1/knowledge-bases \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"证券合规库","description":"合规与制度文档"}'

# 列表 / 详情 / 更新 / 删除
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/knowledge-bases?page=1&page_size=10"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/knowledge-bases/1"
curl -X PUT http://localhost:8080/api/v1/knowledge-bases/1 \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"证券合规库","description":"更新说明"}'
curl -X DELETE http://localhost:8080/api/v1/knowledge-bases/1 \
  -H "Authorization: Bearer <token>"
```

### 分类目录（树形）

```bash
# 在知识库下创建目录（parent_id 可省略表示根目录）
curl -X POST http://localhost:8080/api/v1/knowledge-bases/1/directories \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"合规制度","description":"红线与适当性","sort_order":1}'

curl -X POST http://localhost:8080/api/v1/knowledge-bases/1/directories \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"融资融券","parent_id":1,"sort_order":2}'

# 获取目录树
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/knowledge-bases/1/directories"

# 更新 / 删除目录
curl -X PUT http://localhost:8080/api/v1/directories/1 \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"合规制度","sort_order":1}'
curl -X DELETE http://localhost:8080/api/v1/directories/2 \
  -H "Authorization: Bearer <token>"
```

### 导入文档

支持一次上传多个文件（表单字段 `file` 或 `files` 均可重复出现）。导入时会计算文件内容 MD5，**同一知识库内**已存在相同 MD5 的文件会返回成功并标记 `duplicated=true`，不重复落盘、不触发索引构建。

支持格式：PDF、DOCX、XLSX/XLSM、PPTX、HTML、Markdown、TXT、CSV、JSON、常见图片。不支持旧版 `.doc`。

单次文件数与单文件大小同时受 `rag.max_upload_*` 与租户配额约束。当前限额：`GET /api/v1/system/upload-limits`。

```bash
# 单文件
curl -X POST http://localhost:8080/api/v1/documents/import \
  -H "Authorization: Bearer <token>" \
  -F "file=@./examples/kb_securities_01_compliance.md" \
  -F "title=合规红线" \
  -F "knowledge_base_id=1" \
  -F "directory_id=1"

# 多文件
curl -X POST http://localhost:8080/api/v1/documents/import \
  -H "Authorization: Bearer <token>" \
  -F "file=@./examples/kb_securities_01_compliance.md" \
  -F "file=@./examples/kb_securities_02_suitability.md" \
  -F "knowledge_base_id=1" \
  -F "directory_id=1"
```

未传 `knowledge_base_id` 时，会自动归入该用户的「默认知识库」。新文件导入后状态：`pending` → `indexing` → `ready` / `failed`。`title` 仅在单文件导入时生效；多文件时默认使用各自文件名。

### 查询文档

```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/v1/documents?knowledge_base_id=1&directory_id=1"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/documents/1"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/documents/1/chunks"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/documents/1/index-builds"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/documents/1/recall?k=5"
```

### 删除文档

删除文档会**级联清理**：向量索引中的全部 chunk → 本地上传文件 → PostgreSQL 记录。索引中（`indexing`）的文档会拒绝删除。重新索引前也会先删旧向量，避免残留污染检索。

```bash
# 单文档
curl -X DELETE http://localhost:8080/api/v1/documents/1 \
  -H "Authorization: Bearer <token>"

# 批量
curl -X POST http://localhost:8080/api/v1/documents/delete \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3]}'
```

### 重新索引

对已导入文档重新触发异步索引构建（适用于索引失败重试，或配置变更后重建）。文档 ID 放在请求 body，支持一次多个；不存在、正在 `indexing`、或请求内重复的 ID 会跳过。

```bash
curl -X POST http://localhost:8080/api/v1/documents/reindex \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3]}'
```

### 知识库问答（SSE，带记忆，可按分类过滤）

```bash
curl -N -X POST http://localhost:8080/api/v1/chat/query \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "session_id": "s-demo-001",
    "knowledge_base_id": 1,
    "directory_id": 1,
    "query": "从业人员有哪些合规红线？"
  }'
```

指定 `directory_id` 时会包含该目录及其子目录文档。同一 `session_id` 会自动带上短期/长期对话上下文。SSE 事件：`meta` → `delta` → `done`（含 sources）/ `error`。

配额：`GET /api/v1/chat/quota`（今日剩余新建会话、语音输入、TTS 次数）。

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

### 语音

```bash
# 语音转写（webm / mp3 / wav / m4a / ogg 等）
curl -X POST http://localhost:8080/api/v1/chat/transcribe \
  -H "Authorization: Bearer <token>" \
  -F "file=@./recording.webm"

# 文本朗读（返回音频二进制）
curl -X POST http://localhost:8080/api/v1/chat/speech \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"text":"从业人员不得泄露内幕信息。"}' \
  --output answer.mp3
```

### 历史、会话、反馈

```bash
# 会话列表（需 knowledge_base_id）
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/v1/chat/sessions?knowledge_base_id=1"

# 长期记忆
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/v1/chat/history?session_id=s-demo-001"

# 对助手回答点赞 / 评分（vote 空字符串或 score=0 表示取消该项）
curl -X PUT http://localhost:8080/api/v1/chat/feedback \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"s-demo-001","message_id":12,"vote":"up","score":5}'

# 为用户问题标注相关文档（doc_ids 空数组表示清除）
curl -X PUT http://localhost:8080/api/v1/chat/relevance \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"s-demo-001","message_id":11,"knowledge_base_id":1,"doc_ids":["1"]}'
```

## 配置说明

见 `configs/config.yaml`：

| 区块 | 说明 |
|------|------|
| `server` | 监听地址、Gin mode |
| `log` | zap：level / encoding（console \| json） |
| `jwt` | HS256 密钥、有效期、issuer（生产用 `JWT_SECRET`） |
| `postgres` | 文档导入记录与长期记忆（`connect_timeout_seconds`） |
| `redis` | 短期记忆、索引队列；`provider=redis` 时兼作向量索引 |
| `vector_index` | 向量后端：`redis` / `milvus_lite` |
| `milvus` | `provider=milvus_lite` 时的连接与集合配置 |
| `deepseek` | 对话大模型（`timeout_seconds`） |
| `embedding` | OpenAI 兼容向量模型（超时、拆批、重试、并发） |
| `rag` | 切分 / TopK / Hybrid / Query 改写 / 结构切分 / 引用校验 / 索引队列 / OCR / 上传上限 |
| `rerank` | Cross-Encoder 重排 |
| `asr` | 语音输入转写（SiliconFlow SenseVoice / OpenAI 兼容 transcriptions） |
| `tts` | 问答结果朗读（SiliconFlow CosyVoice / OpenAI 兼容 speech） |
| `memory` | 短期 TTL、消息窗口、摘要超时、token 预算 |
| `agent` | ReAct Agent（enabled / max_steps / tool_top_k） |

## 目录结构

```
cmd/server/          # HTTP 服务入口
cmd/eval/            # Golden 检索评测
cmd/ocr/             # OCR sidecar 入口
configs/             # 本地 / Docker 配置
internal/
  asr/               # 语音转写客户端
  tts/               # 语音合成客户端
  auth/              # JWT 签发与中间件
  config/            # 配置加载与环境变量覆盖
  handler/           # HTTP Handler
  logger/            # zap + Gin 中间件
  memory/            # 短期(Redis) + 长期(PG) 记忆
  model/             # PostgreSQL 模型
  ocr/               # OCR HTTP 服务与引擎
  rag/               # Eino RAG 流水线、解析、索引队列
  repository/        # 数据访问
  server/            # 路由（含 CORS）
  service/           # 业务编排
web/                 # Vue 3 + Arco Design 前端
storage/uploads/     # 上传文件落盘
examples/            # 示例知识库文档与 golden.jsonl
docker-compose.yml   # Postgres / Redis / OCR / Milvus / app / web
```

## 技术栈

- Go + Gin（`gin-contrib/cors`）+ zap
- CloudWeGo Eino / Eino-Ext（DeepSeek、OpenAI Embedding、Redis/Milvus Indexer/Retriever、Recursive Splitter）
- PostgreSQL + GORM
- Redis Stack（短期记忆、索引队列；可选向量检索 + BM25）
- Milvus Standalone（可选，对应 `milvus_lite` 配置）
- Tesseract + poppler（OCR sidecar）
- Vue 3 + Vite + Arco Design + Vue Router + Axios
