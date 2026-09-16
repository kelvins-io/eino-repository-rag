package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Postgres    PostgresConfig    `yaml:"postgres"`
	Redis       RedisConfig       `yaml:"redis"`
	VectorIndex VectorIndexConfig `yaml:"vector_index"`
	Milvus      MilvusConfig      `yaml:"milvus"`
	DeepSeek    DeepSeekConfig    `yaml:"deepseek"`
	Embedding   EmbeddingConfig   `yaml:"embedding"`
	RAG         RAGConfig         `yaml:"rag"`
	Memory      MemoryConfig      `yaml:"memory"`
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
}

type RAGConfig struct {
	ChunkSize   int    `yaml:"chunk_size"`
	OverlapSize int    `yaml:"overlap_size"`
	TopK        int    `yaml:"top_k"`
	UploadDir   string `yaml:"upload_dir"`
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
	if v := os.Getenv("SERVER_ADDR"); v != "" {
		c.Server.Addr = v
	}
	if v := os.Getenv("VECTOR_INDEX_PROVIDER"); v != "" {
		c.VectorIndex.Provider = v
	}
	if v := os.Getenv("MILVUS_ADDRESS"); v != "" {
		c.Milvus.Address = v
	}
}

func (c *Config) setDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "release"
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
	if c.RAG.ChunkSize == 0 {
		c.RAG.ChunkSize = 800
	}
	if c.RAG.TopK == 0 {
		c.RAG.TopK = 5
	}
	if c.RAG.UploadDir == "" {
		c.RAG.UploadDir = "./storage/uploads"
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
}
