package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Log         LogConfig         `yaml:"log"`
	JWT         JWTConfig         `yaml:"jwt"`
	Postgres    PostgresConfig    `yaml:"postgres"`
	Redis       RedisConfig       `yaml:"redis"`
	VectorIndex VectorIndexConfig `yaml:"vector_index"`
	Milvus      MilvusConfig      `yaml:"milvus"`
	DeepSeek    DeepSeekConfig    `yaml:"deepseek"`
	Embedding   EmbeddingConfig   `yaml:"embedding"`
	RAG         RAGConfig         `yaml:"rag"`
	Rerank      RerankConfig      `yaml:"rerank"`
	Memory      MemoryConfig      `yaml:"memory"`
	Agent       AgentConfig       `yaml:"agent"`
}

// AgentConfig ReAct Agent 问答（多步检索）
type AgentConfig struct {
	// Enabled 为 true 时注册 POST /api/v1/chat/agent
	Enabled bool `yaml:"enabled"`
	// MaxSteps 约等于允许的「模型+工具」轮次上限；内部映射为 compose MaxRunSteps = MaxSteps*3
	MaxSteps int `yaml:"max_steps"`
	// ToolTopK knowledge_retrieve 每次召回条数；0 表示使用 rag.top_k
	ToolTopK int `yaml:"tool_top_k"`
}

// JWTConfig 登录签发配置
type JWTConfig struct {
	// Secret HS256 签名密钥；生产环境务必通过 JWT_SECRET 覆盖
	Secret string `yaml:"secret"`
	// ExpireHours token 有效期（小时）
	ExpireHours int `yaml:"expire_hours"`
	// Issuer 签发者标识
	Issuer string `yaml:"issuer"`
}

// LogConfig zap 日志配置
type LogConfig struct {
	// Level: debug | info | warn | error
	Level string `yaml:"level"`
	// Encoding: json | console
	Encoding string `yaml:"encoding"`
	// OutputPaths 输出路径，如 stdout / 文件路径
	OutputPaths []string `yaml:"output_paths"`
	// ErrorOutputPaths 错误输出路径
	ErrorOutputPaths []string `yaml:"error_output_paths"`
}

// VectorIndexProvider 向量索引后端
const (
	VectorIndexRedis      = "redis"
	VectorIndexMilvusLite = "milvus_lite"
)

type VectorIndexConfig struct {
	// Provider: redis | milvus_lite
	Provider string `yaml:"provider"`
}

type MilvusConfig struct {
	// Address Milvus Lite / Standalone gRPC 地址，默认 localhost:19530
	Address    string `yaml:"address"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	Collection string `yaml:"collection"`
	// MetricType: COSINE | L2 | IP
	MetricType string `yaml:"metric_type"`
	// Dimension 向量维度，应与 embedding.dimensions 一致；为 0 时回退 embedding.dimensions
	Dimension int `yaml:"dimension"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
	Mode string `yaml:"mode"`
}

type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

func (c PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

type RedisConfig struct {
	Addr        string `yaml:"addr"`
	Password    string `yaml:"password"`
	DB          int    `yaml:"db"`
	IndexName   string `yaml:"index_name"`
	KeyPrefix   string `yaml:"key_prefix"`
	VectorField string `yaml:"vector_field"`
	VectorDim   int    `yaml:"vector_dim"`
}

type DeepSeekConfig struct {
	APIKey      string  `yaml:"api_key"`
	Model       string  `yaml:"model"`
	BaseURL     string  `yaml:"base_url"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float32 `yaml:"temperature"`
}

type EmbeddingConfig struct {
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	BaseURL    string `yaml:"base_url"`
	Dimensions int    `yaml:"dimensions"`
	// TimeoutSeconds 单次 embedding HTTP 超时；0 表示 120s
	TimeoutSeconds int `yaml:"timeout_seconds"`
	// BatchSize 单次请求的文本条数；Milvus 会把整篇文档一次性 Embed，需在客户端拆批。0 表示 16
	BatchSize int `yaml:"batch_size"`
	// MaxRetries 单批 embedding 瞬时失败（超时/429/5xx）重试次数（不含首次）；0 表示 2
	MaxRetries int `yaml:"max_retries"`
	// MaxConcurrency 全局并发 embedding 请求数，避免多 worker 打满 SiliconFlow。0 表示 2
	MaxConcurrency int `yaml:"max_concurrency"`
}

type RAGConfig struct {
	ChunkSize   int    `yaml:"chunk_size"`
	OverlapSize int    `yaml:"overlap_size"`
	TopK        int    `yaml:"top_k"`
	UploadDir   string `yaml:"upload_dir"`
	// HybridEnabled 启用稠密向量 + BM25 稀疏检索，经 RRF 融合
	HybridEnabled bool `yaml:"hybrid_enabled"`
	// CandidateK Hybrid/Rerank 前每路召回候选数；0 表示 top_k*4
	CandidateK int `yaml:"candidate_k"`
	// RRFK Reciprocal Rank Fusion 常数，经典默认 60
	RRFK int `yaml:"rrf_k"`
	// BM25IndexName Redis 全文索引名（milvus 模式作 sidecar；redis 模式可复用向量索引）
	BM25IndexName string `yaml:"bm25_index_name"`
	// BM25KeyPrefix milvus sidecar 文本索引的 key 前缀
	BM25KeyPrefix string `yaml:"bm25_key_prefix"`

	// IndexWorkers 索引队列并发 worker 数
	IndexWorkers int `yaml:"index_workers"`
	// IndexMaxRetries 单文档索引失败最大重试次数（不含首次）
	IndexMaxRetries int `yaml:"index_max_retries"`
	// IndexJobTimeoutMinutes 单个索引任务超时（分钟）
	IndexJobTimeoutMinutes int `yaml:"index_job_timeout_minutes"`
	// IndexRetryBackoffSeconds 索引失败重试的基础等待；实际等待 = base * 2^(attempt-1)，0 表示 5s
	IndexRetryBackoffSeconds int `yaml:"index_retry_backoff_seconds"`
	// IndexQueueKey Redis 等待队列 List
	IndexQueueKey string `yaml:"index_queue_key"`
	// IndexActiveKey Redis 在途队列 List（BLMOVE 目标，用于崩溃回灌）
	IndexActiveKey string `yaml:"index_active_key"`
	// IndexDLQKey Redis 死信队列 List
	IndexDLQKey string `yaml:"index_dlq_key"`
	// IndexDedupKey Redis 去重 Set（排队/执行中的 doc_id）
	IndexDedupKey string `yaml:"index_dedup_key"`

	// QueryExpandEnabled 启用 LLM Query 改写 / 多路召回（原 query + N 条改写后分别检索再 RRF）
	QueryExpandEnabled bool `yaml:"query_expand_enabled"`
	// QueryExpandN 额外改写条数（不含原 query）；0 表示默认 2
	QueryExpandN int `yaml:"query_expand_n"`
	// QueryExpandTimeoutSeconds 改写 LLM 超时；0 表示 20s
	QueryExpandTimeoutSeconds int `yaml:"query_expand_timeout_seconds"`

	// StructureSplitEnabled 按页/工作表/标题做结构切分，超长段再 Recursive
	StructureSplitEnabled bool `yaml:"structure_split_enabled"`
	// CitationValidateEnabled 生成后校验 [n] 是否落在 sources 范围内，清洗幻觉引用
	CitationValidateEnabled bool `yaml:"citation_validate_enabled"`
	// CitationFilterSources 校验后仅在 done 事件中保留被引用的 sources（默认 false，保留全部召回）
	CitationFilterSources bool `yaml:"citation_filter_sources"`

	// OCR 扫描 PDF / 图片 / 无文字 PPTX·DOCX 的 Tesseract 识别
	OCR OCRConfig `yaml:"ocr"`
}

// OCRConfig 文档解析 OCR（默认调用 compose ocr 服务）
type OCRConfig struct {
	Enabled bool `yaml:"enabled"`
	// Endpoint OCR HTTP 服务，如 http://localhost:18080；为空则回退本机 tesseract
	Endpoint string `yaml:"endpoint"`
	// Languages tesseract -l，默认 chi_sim+eng
	Languages string `yaml:"languages"`
	// DPI pdftoppm 渲染分辨率，默认 200
	DPI int `yaml:"dpi"`
	// Concurrency 同时识别的页数，默认 2
	Concurrency int `yaml:"concurrency"`
	// TimeoutSeconds 单页 tesseract 超时，默认 60
	TimeoutSeconds int `yaml:"timeout_seconds"`
	// PageSegMode tesseract --psm，默认 6（单块文本）
	PageSegMode int `yaml:"page_seg_mode"`
	// TesseractBin 可执行文件名或路径，默认 tesseract（仅 endpoint 为空时）
	TesseractBin string `yaml:"tesseract_bin"`
	// PDFToPPMBin 可执行文件名或路径，默认 pdftoppm（仅 endpoint 为空时）
	PDFToPPMBin string `yaml:"pdftoppm_bin"`
}

// RerankConfig Cross-Encoder / API 重排（OpenAI 兼容，如 SiliconFlow）
type RerankConfig struct {
	Enabled bool   `yaml:"enabled"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
	BaseURL string `yaml:"base_url"`
	// TopN 重排后保留条数；0 表示使用 rag.top_k
	TopN int `yaml:"top_n"`
	// TimeoutSeconds HTTP 超时；0 表示 30s
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

type MemoryConfig struct {
	ShortTermTTLMinutes  int `yaml:"short_term_ttl_minutes"`
	ShortTermMaxMessages int `yaml:"short_term_max_messages"`
	LongTermMaxMessages  int `yaml:"long_term_max_messages"`
	// ContextTokenBudget 注入 prompt 的历史上下文 token 预算（含摘要）；0 表示不限制
	ContextTokenBudget int `yaml:"context_token_budget"`
	// KeepRecentMessages 始终保留的最近原文轮数（摘要时不折叠）
	KeepRecentMessages int `yaml:"keep_recent_messages"`
	// SummaryEnabled 是否启用超出窗口时的历史摘要
	SummaryEnabled bool `yaml:"summary_enabled"`
	// SummaryTriggerMessages 活跃历史超过该条数时，将更早部分折叠为摘要
	SummaryTriggerMessages int `yaml:"summary_trigger_messages"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyEnv()
	cfg.setDefaults()
	return &cfg, nil
}

func (c *Config) applyEnv() {
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" {
		c.DeepSeek.APIKey = v
	}
	if v := os.Getenv("EMBEDDING_API_KEY"); v != "" {
		c.Embedding.APIKey = v
	}
	if v := os.Getenv("POSTGRES_HOST"); v != "" {
		c.Postgres.Host = v
	}
	if v := os.Getenv("POSTGRES_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Postgres.Port = p
		}
	}
	if v := os.Getenv("POSTGRES_USER"); v != "" {
		c.Postgres.User = v
	}
	if v := os.Getenv("POSTGRES_PASSWORD"); v != "" {
		c.Postgres.Password = v
	}
	if v := os.Getenv("POSTGRES_DB"); v != "" {
		c.Postgres.DBName = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		c.Redis.Addr = v
	}
	if v, ok := os.LookupEnv("REDIS_PASSWORD"); ok {
		c.Redis.Password = v
	}
	if v := os.Getenv("SERVER_ADDR"); v != "" {
		c.Server.Addr = v
	}
	if v := os.Getenv("VECTOR_INDEX_PROVIDER"); v != "" {
		c.VectorIndex.Provider = v
	}
	if v := os.Getenv("MILVUS_ADDRESS"); v != "" {
		c.Milvus.Address = v
	}
	if v := os.Getenv("RERANK_API_KEY"); v != "" {
		c.Rerank.APIKey = v
	}
	if v := os.Getenv("RERANK_BASE_URL"); v != "" {
		c.Rerank.BaseURL = v
	}
	if v := os.Getenv("RERANK_MODEL"); v != "" {
		c.Rerank.Model = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.JWT.Secret = v
	}
	if v := os.Getenv("OCR_ENDPOINT"); v != "" {
		c.RAG.OCR.Endpoint = v
	}
}

func (c *Config) setDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "release"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Encoding == "" {
		c.Log.Encoding = "console"
	}
	if len(c.Log.OutputPaths) == 0 {
		c.Log.OutputPaths = []string{"stdout"}
	}
	if len(c.Log.ErrorOutputPaths) == 0 {
		c.Log.ErrorOutputPaths = []string{"stderr"}
	}
	if c.JWT.Secret == "" {
		c.JWT.Secret = "eino-rag-dev-secret-change-me"
	}
	if c.JWT.ExpireHours <= 0 {
		c.JWT.ExpireHours = 72
	}
	if c.JWT.Issuer == "" {
		c.JWT.Issuer = "eino-repository-rag"
	}
	if c.Postgres.SSLMode == "" {
		c.Postgres.SSLMode = "disable"
	}
	if c.Redis.IndexName == "" {
		c.Redis.IndexName = "kb_index"
	}
	if c.Redis.KeyPrefix == "" {
		c.Redis.KeyPrefix = "kb:doc:"
	}
	if c.Redis.VectorField == "" {
		c.Redis.VectorField = "content_vector"
	}
	if c.Redis.VectorDim == 0 {
		c.Redis.VectorDim = 1536
	}
	if c.VectorIndex.Provider == "" {
		c.VectorIndex.Provider = VectorIndexRedis
	}
	if c.Milvus.Address == "" {
		c.Milvus.Address = "localhost:19530"
	}
	if c.Milvus.Collection == "" {
		c.Milvus.Collection = "kb_docs"
	}
	if c.Milvus.MetricType == "" {
		c.Milvus.MetricType = "COSINE"
	}
	if c.DeepSeek.Model == "" {
		c.DeepSeek.Model = "deepseek-chat"
	}
	if c.DeepSeek.BaseURL == "" {
		c.DeepSeek.BaseURL = "https://api.deepseek.com"
	}
	if c.DeepSeek.MaxTokens == 0 {
		c.DeepSeek.MaxTokens = 2048
	}
	if c.Embedding.Model == "" {
		c.Embedding.Model = "text-embedding-3-small"
	}
	// Embedding.Dimensions 为 0 表示不向 API 传 dimensions（SiliconFlow 的 BAAI/bge-m3 等会因此返回 20015）
	if c.Embedding.TimeoutSeconds <= 0 {
		c.Embedding.TimeoutSeconds = 120
	}
	if c.Embedding.BatchSize <= 0 {
		c.Embedding.BatchSize = 16
	}
	if c.Embedding.MaxRetries <= 0 {
		c.Embedding.MaxRetries = 2
	}
	if c.Embedding.MaxConcurrency <= 0 {
		c.Embedding.MaxConcurrency = 2
	}
	if c.RAG.ChunkSize == 0 {
		c.RAG.ChunkSize = 800
	}
	if c.RAG.TopK == 0 {
		c.RAG.TopK = 5
	}
	if c.RAG.UploadDir == "" {
		c.RAG.UploadDir = "./storage/uploads"
	}
	if c.RAG.RRFK <= 0 {
		c.RAG.RRFK = 60
	}
	if c.RAG.BM25IndexName == "" {
		c.RAG.BM25IndexName = "kb_bm25_index"
	}
	if c.RAG.BM25KeyPrefix == "" {
		c.RAG.BM25KeyPrefix = "kb:bm25:"
	}
	if c.RAG.IndexWorkers <= 0 {
		c.RAG.IndexWorkers = 2
	}
	if c.RAG.IndexMaxRetries <= 0 {
		c.RAG.IndexMaxRetries = 3
	}
	if c.RAG.IndexJobTimeoutMinutes <= 0 {
		c.RAG.IndexJobTimeoutMinutes = 10
	}
	if c.RAG.IndexRetryBackoffSeconds <= 0 {
		c.RAG.IndexRetryBackoffSeconds = 5
	}
	if c.RAG.IndexQueueKey == "" {
		c.RAG.IndexQueueKey = "kb:index:queue"
	}
	if c.RAG.IndexActiveKey == "" {
		c.RAG.IndexActiveKey = "kb:index:active"
	}
	if c.RAG.IndexDLQKey == "" {
		c.RAG.IndexDLQKey = "kb:index:dlq"
	}
	if c.RAG.IndexDedupKey == "" {
		c.RAG.IndexDedupKey = "kb:index:queued"
	}
	if c.RAG.QueryExpandN <= 0 {
		c.RAG.QueryExpandN = 2
	}
	if c.RAG.QueryExpandTimeoutSeconds <= 0 {
		c.RAG.QueryExpandTimeoutSeconds = 20
	}
	if c.RAG.OCR.Languages == "" {
		c.RAG.OCR.Languages = "chi_sim+eng"
	}
	if c.RAG.OCR.DPI <= 0 {
		c.RAG.OCR.DPI = 200
	}
	if c.RAG.OCR.Concurrency <= 0 {
		c.RAG.OCR.Concurrency = 1
	}
	if c.RAG.OCR.TimeoutSeconds <= 0 {
		c.RAG.OCR.TimeoutSeconds = 60
	}
	if c.RAG.OCR.PageSegMode <= 0 {
		c.RAG.OCR.PageSegMode = 6
	}
	if c.RAG.OCR.TesseractBin == "" {
		c.RAG.OCR.TesseractBin = "tesseract"
	}
	if c.RAG.OCR.PDFToPPMBin == "" {
		c.RAG.OCR.PDFToPPMBin = "pdftoppm"
	}
	if c.Rerank.APIKey == "" {
		c.Rerank.APIKey = c.Embedding.APIKey
	}
	if c.Rerank.BaseURL == "" {
		c.Rerank.BaseURL = c.Embedding.BaseURL
	}
	if c.Rerank.Model == "" {
		c.Rerank.Model = "BAAI/bge-reranker-v2-m3"
	}
	if c.Rerank.TimeoutSeconds <= 0 {
		c.Rerank.TimeoutSeconds = 30
	}
	if c.Memory.ShortTermTTLMinutes == 0 {
		c.Memory.ShortTermTTLMinutes = 60
	}
	if c.Memory.ShortTermMaxMessages == 0 {
		c.Memory.ShortTermMaxMessages = 20
	}
	if c.Memory.LongTermMaxMessages == 0 {
		c.Memory.LongTermMaxMessages = 50
	}
	if c.Memory.ContextTokenBudget == 0 {
		c.Memory.ContextTokenBudget = 3000
	}
	if c.Memory.KeepRecentMessages == 0 {
		c.Memory.KeepRecentMessages = 6
	}
	if c.Memory.SummaryTriggerMessages == 0 {
		c.Memory.SummaryTriggerMessages = 8
	}
	if c.Agent.MaxSteps <= 0 {
		c.Agent.MaxSteps = 4
	}
	if c.Agent.ToolTopK <= 0 {
		c.Agent.ToolTopK = c.RAG.TopK
		if c.Agent.ToolTopK <= 0 {
			c.Agent.ToolTopK = 5
		}
	}
}
