# eino-repository-rag

[English](README.en.md) | [中文](README.md)

Enterprise knowledge-base RAG service built on [CloudWeGo Eino](https://github.com/cloudwego/eino): multi-tenant JWT auth, document ingest with async indexing, hybrid retrieval, streaming Q&A, and a Vue 3 admin UI with Chinese/English locales.

Frontend details: [web/README.en.md](web/README.en.md).

## Features

- **Multi-tenant auth**: tenants + user register/login; JWT (HS256); identity comes from the token, clients cannot forge `user_id`
  - Startup ensures tenant `default` and its `admin` (initial password is logged once)
  - Self-registration is blocked on `default`; other tenants can register
  - Platform admin (`default` / `admin`) can create tenants, view quotas, and update limits
  - Each tenant `admin` can enable/disable login for users in that tenant
  - Login and register accept `?tenant_id=` to prefill the tenant ID, and default to `guest` when it is absent (platform admin still uses tenant `default`)
- **Tenant quotas**: total files / max file size / new sessions per day / turns per session / voice inputs per day / TTS per day (defaults are all 5)
- **Document import**: `POST /api/v1/documents/import`, multiple files per request; MD5 dedup within a knowledge base; async vector indexing via a **Redis index queue** (rate limit, retries, crash recovery); **the local upload is deleted after a successful index**, so the document cannot be reindexed afterward (failed jobs keep the source file for retry)
- **Parsing / OCR**: PDF, Office (DOCX/XLSX/PPTX), Markdown/HTML/text/CSV/JSON, images; scanned PDFs / images / text-less PPTX·DOCX fall back to Tesseract (compose `ocr` service; no local brew required)
- **Knowledge bases and directories**: multiple KBs + directory tree; import targeting, list filters, retrieval filters (including children); KBs are readable across the tenant
- **Vector search**: `redis` or `milvus_lite` (Eino Indexer/Retriever + OpenAI-compatible embedding)
- **Hybrid retrieval**: dense vectors + Redis BM25 (RRF fusion); `milvus` mode maintains a BM25 sidecar
- **Query rewrite / multi-query recall**: LLM produces N rewrites, each is retrieved with the original query, then RRF; rerank still uses the original question
- **Structure-aware split**: PDF/PPTX by page, XLSX by sheet, Markdown/HTML by heading; oversize segments go through Recursive split, keeping `page`/`section` meta
- **Citation post-check**: after generation, drop `[n]` citations that are not in `sources`
- **Agent / workflow**: Eino ReAct + `knowledge_retrieve` multi-step retrieval (`POST /api/v1/chat/agent`); linear RAG uses `/chat/query` (both SSE)
- **Rerank**: OpenAI-compatible cross-encoder (e.g. SiliconFlow `BAAI/bge-reranker-v2-m3`)
- **Speech**: ASR (SenseVoice / OpenAI-compatible transcriptions) + TTS (CosyVoice / OpenAI-compatible speech)
- **Q&A feedback**: upvote/downvote/1–5 score on assistant answers; label relevant docs on user questions for document recall
- **Retrieval observability**: chunk list, index-build history, document recall (Hit@K), citation counts
- **Golden eval**: JSONL set + Hit@K / Recall@K / MRR (`cmd/eval`)
- **LLM answers**: DeepSeek (`eino-ext/components/model/deepseek`)
- **Memory**
  - Short-term: Redis List (per session, with TTL)
  - Long-term: PostgreSQL `messages` / `conversations`
  - Compression: rolling LLM summary of older turns when over message/token budget, then trim to the token budget before prompt injection

## Architecture

```
Client
  │
  ▼
HTTP API (Gin) + JWT
  ├─ Public: register / login / health
  ├─ Admin (authenticated)
  │    ├─ Platform admin: create tenant / tenant list / quotas
  │    └─ Tenant admin: user list / toggle login
  ├─ Document import → disk + PostgreSQL row → Redis index queue → Index Pipeline
  │                                      ├─ Parser (PDF/Office/text/image OCR)
  │                                      ├─ Recursive / structure split
  │                                      ├─ Embedding
  │                                      ├─ Vector Store (redis / milvus_lite)
  │                                      ├─ BM25 sidecar (hybrid + milvus)
  │                                      └─ delete local upload on success
  └─ Q&A retrieval → (optional) query rewrite, multi-query recall
               → short/long-term memory
               → Dense Retriever + BM25 (optional Hybrid/RRF)
               → cross-query RRF → Rerank (optional)
               → DeepSeek Generate → citation check → persist memory
  └─ Agent Q&A → ReAct (multi-step knowledge_retrieve) → citation check → persist memory
  └─ Speech → ASR for questions / TTS for answers (counts toward daily tenant quota)
```

Index queue keys: `kb:index:queue` (pending) / `kb:index:active` (in-flight, requeued on startup) / `kb:index:dlq` (dead letter). `rag.index_workers` sets concurrency; failures retry up to `index_max_retries`.

## Quick start

### 1. Start dependencies

```bash
# Default: PostgreSQL + Redis Stack + OCR sidecar
docker compose up -d

# For milvus_lite vector index, also start local Milvus Standalone
docker compose --profile milvus up -d
```

PostgreSQL is mapped to `15433`, Redis to `16379`, OCR to `18080`. Local `configs/config.yaml` defaults to `vector_index.provider: milvus_lite`; the Docker app image defaults to `redis`.

### 2. Environment variables

```bash
cp .env.example .env
# Edit .env and set DEEPSEEK_API_KEY and EMBEDDING_API_KEY
```

> Containers also honor `REDIS_PASSWORD` (empty value can override local yaml), `JWT_SECRET`, `VECTOR_INDEX_PROVIDER`, `MILVUS_ADDRESS`, and more.

> DeepSeek does not ship embeddings. Configure an OpenAI-compatible embedding API (OpenAI / SiliconFlow / Tongyi, etc.).
> Dimensions must match the vector store: Redis uses `redis.vector_dim`, Milvus uses `milvus.dimension` (`BAAI/bge-m3` is 1024). Do not send `embedding.dimensions` for bge-m3 (SiliconFlow returns 20015).

### 3. Choose a vector backend

`configs/config.yaml`:

```yaml
vector_index:
  provider: "milvus_lite"   # or redis

milvus:
  address: "localhost:19530"
  collection: "kb_docs"
  metric_type: "COSINE"
  dimension: 1024
```

Env overrides: `VECTOR_INDEX_PROVIDER=milvus_lite`, `MILVUS_ADDRESS=localhost:19530`.

> Official Milvus Lite (local `.db` file) is mainly a Python story. This Go service talks to local Milvus Standalone over gRPC; the config name `milvus_lite` is for lightweight local deploy.

### 3.1 Hybrid + rerank (optional, on by default)

```yaml
rag:
  hybrid_enabled: true   # dense vectors + BM25, fused with RRF
  candidate_k: 20        # candidates per path; 0 = top_k*4
  rrf_k: 60

rerank:
  enabled: true
  model: "BAAI/bge-reranker-v2-m3"
  base_url: "https://api.siliconflow.cn/v1"
  # empty api_key reuses embedding.api_key
  top_n: 5
```

Env: `RERANK_API_KEY`, `RERANK_BASE_URL`, `RERANK_MODEL`.

> After turning Hybrid on for milvus, **reindex** existing documents so the BM25 sidecar is populated.
> Redis vector mode reuses `content`/`title` TEXT fields on the index; no extra sync.

### 3.2 Query rewrite / multi-query recall (optional, on by default)

```yaml
rag:
  query_expand_enabled: true
  query_expand_n: 2                 # extra rewrites (excluding the original)
  query_expand_timeout_seconds: 20
```

Disable with `query_expand_enabled: false`. On rewrite failure the service falls back to a single original-query retrieve.

### 3.3 Structure split + citation post-check

```yaml
rag:
  structure_split_enabled: true   # split by page/sheet/heading
  citation_validate_enabled: true # strip out-of-range [n]
  citation_filter_sources: false  # if true, done keeps only cited sources
```

Reindex existing documents after changing structure split. In streaming, `delta` is raw generation; `done.answer` and persisted memory use the validated text.

### 3.4 Golden eval (retrieval regression)

Import `examples/kb_securities_*.md`, wait until indexed, edit `examples/eval/golden.jsonl` with real `knowledge_base_id` / `tenant_id`, then:

```bash
make eval
# or
go run ./cmd/eval -config configs/config.yaml \
  -golden examples/eval/golden.jsonl -k 5 -out /tmp/eval-report.json
```

Reports Hit@K / Recall@K / MRR. Label relevance with any of `relevant_contains` (content substring), `relevant_doc_ids`, or `relevant_titles`.

### 3.5 Agent multi-step retrieval (optional)

```yaml
agent:
  enabled: true   # false: do not register /chat/agent
  max_steps: 4    # roughly max retrieve rounds (internal MaxRunSteps = max_steps*3)
  tool_top_k: 5
```

Unlike linear RAG (`/chat/query`, one retrieve), Agent uses Eino ReAct and may call `knowledge_retrieve` several times (still Hybrid/Expand/Rerank). SSE also emits `step` / `tool_start` / `tool_result`. Higher latency and cost; better for multi-hop questions.

### 3.6 Scanned docs / image OCR

The parser first extracts embedded PDF text. **Image pages, scans, garbled CJK PDFs**, and **PNG/JPG images** fall back to Tesseract. Text-less PPTX slides / DOCX files OCR images under `media`.

OCR runs in the compose `ocr` service (tesseract + Chinese lang pack + poppler). You do **not** need `brew install` locally.

```yaml
rag:
  ocr:
    enabled: true
    endpoint: "http://localhost:18080"  # inside compose: http://ocr:8080
    languages: "chi_sim+eng"
    dpi: 200
    concurrency: 1
    timeout_seconds: 60
    page_seg_mode: 6
  index_job_timeout_minutes: 30
```

```bash
docker compose up -d ocr
# or with other deps: docker compose up -d
```

`OCR_ENDPOINT` overrides the endpoint. Empty endpoint falls back to local `tesseract`/`pdftoppm`. Disable with `rag.ocr.enabled: false`.

### 3.7 Voice input / answer speech (optional, on by default)

```yaml
asr:
  enabled: true
  model: "FunAudioLLM/SenseVoiceSmall"
  base_url: "https://api.siliconflow.cn/v1"
  language: "zh"
  max_audio_mb: 8
  # empty api_key reuses embedding.api_key

tts:
  enabled: true
  model: "FunAudioLLM/CosyVoice2-0.5B"
  voice: "anna"          # bella, alex, etc.
  max_chars: 4000
  # empty api_key reuses asr / embedding
```

Env: `ASR_API_KEY`, `ASR_BASE_URL`, `ASR_MODEL`, `TTS_API_KEY`, `TTS_BASE_URL`, `TTS_MODEL`, `TTS_VOICE`. Disable with `asr.enabled: false` / `tts.enabled: false`.

### 4. Start the service

Local (Go 1.26+):

```bash
go run ./cmd/server -config configs/config.yaml
# or
make run
```

On first boot the `default` tenant `admin` password is printed in logs (once, when created). Override the default JWT secret with `JWT_SECRET` in production.

Docker (build backend + frontend and start with deps; default vector store is Redis):

```bash
make docker-up
# equivalent: docker compose --profile app up -d --build
# images only: make docker-build
```

- API: http://localhost:8080
- Web: http://localhost:5173 (nginx static files; `/api` and `/health` proxied to `app`)

Milvus mode:

```bash
VECTOR_INDEX_PROVIDER=milvus_lite docker compose --profile app --profile milvus up -d --build
```

The backend image uses `configs/config.docker.yaml` (hostnames `postgres`/`redis`, JSON logs, `upload_dir=/app/storage/uploads`). Listens on `:8080` with CORS enabled.

### 5. Auth (tenant + JWT)

Startup ensures tenant `default` and admin `admin`. Business APIs require `Authorization: Bearer <token>`.

```bash
# Log in with the default/admin password from logs
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","username":"admin","password":"<initial password from logs>"}'
# data.token → later requests: Authorization: Bearer <token>

# Current user
curl http://localhost:8080/api/v1/auth/me \
  -H "Authorization: Bearer <token>"
```

**Platform admin** (`admin` of tenant `default`) can create tenants. A new tenant gets an `admin` user; the password is logged only, the response includes `admin_username`.

```bash
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"code":"acme","name":"Acme Inc."}'
```

Other tenants can self-register (`default` cannot):

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"acme","username":"alice","password":"secret1"}'
```

### 6. Start the web UI (optional)

Split deploy: API `:8080`, Vite dev server `:5173`.

```bash
cd web
npm install
npm run dev
# or from repo root: make web-install && make web
```

Open http://localhost:5173 . In development Vite proxies `/api` and `/health` to the backend. Production builds use `VITE_API_BASE` from `web/.env.production` and talk to the API directly (needs CORS).

The Docker frontend image (`web/Dockerfile`) builds with `VITE_API_BASE=` (same origin); nginx reverse-proxies the API, so you do not need to change `.env.production`.

UI: login/register (`?tenant_id=` prefills the tenant ID, otherwise `guest`), Chinese/English locale switch, knowledge-base CRUD, directory tree, document import/list/delete/reindex/chunks/recall, Q&A with session memory (standard RAG / Agent), voice in/out, answer feedback, user management; platform admin also gets tenant management and quotas. Documents whose source file was removed after a successful index cannot be reindexed. Platform admin login still requires tenant ID `default`.

See [web/README.en.md](web/README.en.md) for frontend-only docs.

## API

Unless noted, send `Authorization: Bearer <token>`. `user_id` / `tenant_id` are injected from JWT; do not forge them in the body.

### Auth and admin

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/register` | Register (public; rejected for `default`) |
| POST | `/api/v1/auth/login` | Login (public) |
| GET | `/api/v1/auth/me` | Current user |
| GET | `/api/v1/users` | Users in this tenant |
| PUT | `/api/v1/users/login-enabled` | Tenant admin toggles login |
| POST | `/api/v1/tenants` | Platform admin creates a tenant |
| GET | `/api/v1/tenants` | Platform admin tenant list and usage |
| PUT | `/api/v1/tenants/limits` | Platform admin updates quotas |
| GET | `/health` | Health (public) |

```bash
# Tenant admin disables a user
curl -X PUT http://localhost:8080/api/v1/users/login-enabled \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","enabled":false}'

# Update tenant quotas
curl -X PUT http://localhost:8080/api/v1/tenants/limits \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"code":"acme","max_files":20,"max_file_size_mb":50,"max_sessions":20,"max_turns":30,"max_voice_inputs":20,"max_tts":20}'
```

### Knowledge bases

```bash
# Create
curl -X POST http://localhost:8080/api/v1/knowledge-bases \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Securities compliance","description":"Policies and procedures"}'

# List / get / update / delete
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/knowledge-bases?page=1&page_size=10"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/knowledge-bases/1"
curl -X PUT http://localhost:8080/api/v1/knowledge-bases/1 \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Securities compliance","description":"Updated notes"}'
curl -X DELETE http://localhost:8080/api/v1/knowledge-bases/1 \
  -H "Authorization: Bearer <token>"
```

### Directory tree

```bash
# Create under a KB (omit parent_id for a root directory)
curl -X POST http://localhost:8080/api/v1/knowledge-bases/1/directories \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Compliance","description":"Red lines and suitability","sort_order":1}'

curl -X POST http://localhost:8080/api/v1/knowledge-bases/1/directories \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Margin trading","parent_id":1,"sort_order":2}'

# Tree
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/knowledge-bases/1/directories"

# Update / delete
curl -X PUT http://localhost:8080/api/v1/directories/1 \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Compliance","sort_order":1}'
curl -X DELETE http://localhost:8080/api/v1/directories/2 \
  -H "Authorization: Bearer <token>"
```

### Import documents

Multiple files per request (`file` or `files`, repeatable). Content MD5 is computed; **within the same knowledge base** an existing MD5 returns success with `duplicated=true` and does not write to disk or enqueue indexing.

Formats: PDF, DOCX, XLSX/XLSM, PPTX, HTML, Markdown, TXT, CSV, JSON, common images. Legacy `.doc` is not supported.

Per-request file count and size are capped by both `rag.max_upload_*` and the tenant quota. Current limits: `GET /api/v1/system/upload-limits`.

```bash
# Single file
curl -X POST http://localhost:8080/api/v1/documents/import \
  -H "Authorization: Bearer <token>" \
  -F "file=@./examples/kb_securities_01_compliance.md" \
  -F "title=Compliance red lines" \
  -F "knowledge_base_id=1" \
  -F "directory_id=1"

# Multiple files
curl -X POST http://localhost:8080/api/v1/documents/import \
  -H "Authorization: Bearer <token>" \
  -F "file=@./examples/kb_securities_01_compliance.md" \
  -F "file=@./examples/kb_securities_02_suitability.md" \
  -F "knowledge_base_id=1" \
  -F "directory_id=1"
```

If `knowledge_base_id` is omitted, files go to the user's default knowledge base. New files: `pending` → `indexing` → `ready` / `failed`. `title` applies only to single-file imports; multi-file uses each filename.

After a successful index the local upload is deleted and `file_path` is cleared; list/detail responses set `source_available=false`. Failed jobs keep the source file so you can retry.

### Query documents

```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/v1/documents?knowledge_base_id=1&directory_id=1"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/documents/1"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/documents/1/chunks"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/documents/1/index-builds"
curl -H "Authorization: Bearer <token>" "http://localhost:8080/api/v1/documents/1/recall?k=5"
```

### Delete documents

Delete **cascades**: all vector chunks → remaining local upload (if any) → PostgreSQL row. Documents in `indexing` cannot be deleted. Reindex also drops old vectors first to avoid stale hits. Successfully indexed documents usually no longer have a local source file.

```bash
# One
curl -X DELETE http://localhost:8080/api/v1/documents/1 \
  -H "Authorization: Bearer <token>"

# Batch
curl -X POST http://localhost:8080/api/v1/documents/delete \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3]}'
```

### Reindex

Re-enqueue async index builds (retry failures, or rebuild after config changes while the source file still exists). IDs in the body; missing, currently `indexing`, source already cleaned (`source_available=false`), or duplicate IDs in the request are skipped.

```bash
curl -X POST http://localhost:8080/api/v1/documents/reindex \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"ids":[1,2,3]}'
```

### Knowledge Q&A (SSE, with memory, optional directory filter)

```bash
curl -N -X POST http://localhost:8080/api/v1/chat/query \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "session_id": "s-demo-001",
    "knowledge_base_id": 1,
    "directory_id": 1,
    "query": "What compliance red lines apply to practitioners?"
  }'
```

`directory_id` includes that directory and its descendants. The same `session_id` carries short/long-term context. SSE events: `meta` → `delta` → `done` (with sources) / `error`.

Quota: `GET /api/v1/chat/quota` (remaining new sessions, voice inputs, and TTS for today).

### Agent multi-step Q&A

Requires `agent.enabled: true`. Body matches `/chat/query`:

```bash
curl -N -X POST http://localhost:8080/api/v1/chat/agent \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "s-agent-001",
    "knowledge_base_id": 1,
    "query": "Compare what compliance red lines vs suitability assessment each focus on."
  }'
```

SSE events: `meta` → `step` / `tool_start` / `tool_result` (multiple rounds) → `delta` → `done` (with sources).

### Speech

```bash
# Transcribe (webm / mp3 / wav / m4a / ogg, etc.)
curl -X POST http://localhost:8080/api/v1/chat/transcribe \
  -H "Authorization: Bearer <token>" \
  -F "file=@./recording.webm"

# Synthesize (binary audio)
curl -X POST http://localhost:8080/api/v1/chat/speech \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"text":"Practitioners must not leak insider information."}' \
  --output answer.mp3
```

### History, sessions, feedback

```bash
# Sessions (knowledge_base_id required)
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/v1/chat/sessions?knowledge_base_id=1"

# Long-term memory
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/v1/chat/history?session_id=s-demo-001"

# Upvote / score an assistant message (empty vote or score=0 clears that field)
curl -X PUT http://localhost:8080/api/v1/chat/feedback \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"s-demo-001","message_id":12,"vote":"up","score":5}'

# Label relevant docs for a user question (empty doc_ids clears)
curl -X PUT http://localhost:8080/api/v1/chat/relevance \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"s-demo-001","message_id":11,"knowledge_base_id":1,"doc_ids":["1"]}'
```

## Configuration

See `configs/config.yaml`:

| Block | Purpose |
|-------|---------|
| `server` | Listen address, Gin mode |
| `log` | zap: level / encoding (`console` \| `json`) |
| `jwt` | HS256 secret, TTL, issuer (use `JWT_SECRET` in production) |
| `postgres` | Import records and long-term memory (`connect_timeout_seconds`) |
| `redis` | Short-term memory, index queue; also the vector store when `provider=redis` |
| `vector_index` | Vector backend: `redis` / `milvus_lite` |
| `milvus` | Connection and collection when `provider=milvus_lite` |
| `deepseek` | Chat LLM (`timeout_seconds`) |
| `embedding` | OpenAI-compatible embeddings (timeout, batching, retries, concurrency) |
| `rag` | Split / TopK / Hybrid / query rewrite / structure split / citation check / index queue / OCR / upload caps |
| `rerank` | Cross-encoder rerank |
| `asr` | Speech-to-text (SiliconFlow SenseVoice / OpenAI-compatible transcriptions) |
| `tts` | Answer speech (SiliconFlow CosyVoice / OpenAI-compatible speech) |
| `memory` | Short-term TTL, window, summary timeout, token budget |
| `agent` | ReAct Agent (`enabled` / `max_steps` / `tool_top_k`) |

## Layout

```
cmd/server/          # HTTP server
cmd/eval/            # Golden retrieval eval
cmd/ocr/             # OCR sidecar
configs/             # Local / Docker config
internal/
  asr/               # Speech-to-text client
  tts/               # Speech synthesis client
  auth/              # JWT issue and middleware
  config/            # Config load and env overrides
  handler/           # HTTP handlers
  logger/            # zap + Gin middleware
  memory/            # Short-term (Redis) + long-term (PG)
  model/             # PostgreSQL models
  ocr/               # OCR HTTP service and engine
  rag/               # Eino RAG pipeline, parsers, index queue
  repository/        # Data access
  server/            # Routes (incl. CORS)
  service/           # Business logic
web/                 # Vue 3 + Element Plus UI
storage/uploads/     # Uploaded files
examples/            # Sample KB docs and golden.jsonl
docker-compose.yml   # Postgres / Redis / OCR / Milvus / app / web
```

## Stack

- Go + Gin (`gin-contrib/cors`) + zap
- CloudWeGo Eino / Eino-Ext (DeepSeek, OpenAI embedding, Redis/Milvus indexer/retriever, recursive splitter)
- PostgreSQL + GORM
- Redis Stack (short-term memory, index queue; optional vector search + BM25)
- Milvus Standalone (optional, `milvus_lite` config)
- Tesseract + poppler (OCR sidecar)
- Vue 3 + Vite + Element Plus + Vue Router + Vue I18n + Axios
