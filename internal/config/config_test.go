package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetDefaultsTimeouts(t *testing.T) {
	var c Config
	c.setDefaults()
	if c.DeepSeek.TimeoutSeconds != 120 {
		t.Fatalf("deepseek timeout=%d", c.DeepSeek.TimeoutSeconds)
	}
	if c.Embedding.TimeoutSeconds != 120 {
		t.Fatalf("embedding timeout=%d", c.Embedding.TimeoutSeconds)
	}
	if c.Rerank.TimeoutSeconds != 30 {
		t.Fatalf("rerank timeout=%d", c.Rerank.TimeoutSeconds)
	}
	if c.ASR.TimeoutSeconds != 60 {
		t.Fatalf("asr timeout=%d", c.ASR.TimeoutSeconds)
	}
	if c.ASR.MaxAudioMB != 8 {
		t.Fatalf("asr max audio mb=%d", c.ASR.MaxAudioMB)
	}
	if c.ASR.Model != "FunAudioLLM/SenseVoiceSmall" {
		t.Fatalf("asr model=%s", c.ASR.Model)
	}
	if c.TTS.TimeoutSeconds != 60 {
		t.Fatalf("tts timeout=%d", c.TTS.TimeoutSeconds)
	}
	if c.TTS.MaxChars != 4000 {
		t.Fatalf("tts max chars=%d", c.TTS.MaxChars)
	}
	if c.RAG.OCR.TimeoutSeconds != 60 {
		t.Fatalf("ocr timeout=%d", c.RAG.OCR.TimeoutSeconds)
	}
	if c.RAG.MaxUploadFileSizeMB != 50 {
		t.Fatalf("max upload file size mb=%d", c.RAG.MaxUploadFileSizeMB)
	}
	if c.RAG.MaxUploadFiles != 20 {
		t.Fatalf("max upload files=%d", c.RAG.MaxUploadFiles)
	}
	if c.RAG.QueryExpandTimeoutSeconds != 20 {
		t.Fatalf("expand timeout=%d", c.RAG.QueryExpandTimeoutSeconds)
	}
	if c.Memory.SummaryTimeoutSeconds != 30 {
		t.Fatalf("summary timeout=%d", c.Memory.SummaryTimeoutSeconds)
	}
	if c.Postgres.ConnectTimeoutSeconds != 10 {
		t.Fatalf("postgres connect=%d", c.Postgres.ConnectTimeoutSeconds)
	}
	if c.Redis.DialTimeoutSeconds != 5 || c.Redis.ReadTimeoutSeconds != 5 || c.Redis.WriteTimeoutSeconds != 5 {
		t.Fatalf("redis timeouts dial=%d read=%d write=%d",
			c.Redis.DialTimeoutSeconds, c.Redis.ReadTimeoutSeconds, c.Redis.WriteTimeoutSeconds)
	}
	if c.Milvus.ConnectTimeoutSeconds != 10 {
		t.Fatalf("milvus connect=%d", c.Milvus.ConnectTimeoutSeconds)
	}
	dsn := c.Postgres.DSN()
	if !strings.Contains(dsn, "connect_timeout=10") {
		t.Fatalf("dsn=%s", dsn)
	}
	if c.Redis.DialTimeout() != 5*time.Second || c.Milvus.ConnectTimeout() != 10*time.Second {
		t.Fatalf("duration helpers dial=%s milvus=%s", c.Redis.DialTimeout(), c.Milvus.ConnectTimeout())
	}
}

func TestApplyEnvOverridesTimeouts(t *testing.T) {
	t.Setenv("DEEPSEEK_TIMEOUT_SECONDS", "180")
	t.Setenv("EMBEDDING_TIMEOUT_SECONDS", "90")
	t.Setenv("RERANK_TIMEOUT_SECONDS", "45")
	t.Setenv("ASR_TIMEOUT_SECONDS", "70")
	t.Setenv("ASR_MAX_AUDIO_MB", "12")
	t.Setenv("OCR_TIMEOUT_SECONDS", "75")
	t.Setenv("QUERY_EXPAND_TIMEOUT_SECONDS", "15")
	t.Setenv("MEMORY_SUMMARY_TIMEOUT_SECONDS", "25")
	t.Setenv("POSTGRES_CONNECT_TIMEOUT_SECONDS", "8")
	t.Setenv("REDIS_DIAL_TIMEOUT_SECONDS", "3")
	t.Setenv("REDIS_READ_TIMEOUT_SECONDS", "7")
	t.Setenv("REDIS_WRITE_TIMEOUT_SECONDS", "6")
	t.Setenv("MILVUS_CONNECT_TIMEOUT_SECONDS", "12")

	c := Config{
		DeepSeek:  DeepSeekConfig{TimeoutSeconds: 120},
		Embedding: EmbeddingConfig{TimeoutSeconds: 120},
		Rerank:    RerankConfig{TimeoutSeconds: 30},
		ASR:       ASRConfig{TimeoutSeconds: 60, MaxAudioMB: 8},
	}
	c.RAG.OCR.TimeoutSeconds = 60
	c.RAG.QueryExpandTimeoutSeconds = 20
	c.Memory.SummaryTimeoutSeconds = 30
	c.Postgres.ConnectTimeoutSeconds = 10
	c.Redis.DialTimeoutSeconds = 5
	c.Redis.ReadTimeoutSeconds = 5
	c.Redis.WriteTimeoutSeconds = 5
	c.Milvus.ConnectTimeoutSeconds = 10
	c.applyEnv()
	if c.DeepSeek.TimeoutSeconds != 180 || c.Embedding.TimeoutSeconds != 90 ||
		c.Rerank.TimeoutSeconds != 45 || c.ASR.TimeoutSeconds != 70 || c.ASR.MaxAudioMB != 12 ||
		c.RAG.OCR.TimeoutSeconds != 75 ||
		c.RAG.QueryExpandTimeoutSeconds != 15 || c.Memory.SummaryTimeoutSeconds != 25 ||
		c.Postgres.ConnectTimeoutSeconds != 8 || c.Redis.DialTimeoutSeconds != 3 ||
		c.Redis.ReadTimeoutSeconds != 7 || c.Redis.WriteTimeoutSeconds != 6 ||
		c.Milvus.ConnectTimeoutSeconds != 12 {
		t.Fatalf("env override failed postgres=%d redis=%d/%d/%d milvus=%d",
			c.Postgres.ConnectTimeoutSeconds, c.Redis.DialTimeoutSeconds,
			c.Redis.ReadTimeoutSeconds, c.Redis.WriteTimeoutSeconds, c.Milvus.ConnectTimeoutSeconds)
	}
}

func TestLoadYAMLTimeouts(t *testing.T) {
	t.Setenv("DEEPSEEK_TIMEOUT_SECONDS", "")
	t.Setenv("EMBEDDING_TIMEOUT_SECONDS", "")
	t.Setenv("RERANK_TIMEOUT_SECONDS", "")
	t.Setenv("ASR_TIMEOUT_SECONDS", "")
	t.Setenv("ASR_MAX_AUDIO_MB", "")
	t.Setenv("OCR_TIMEOUT_SECONDS", "")
	t.Setenv("QUERY_EXPAND_TIMEOUT_SECONDS", "")
	t.Setenv("MEMORY_SUMMARY_TIMEOUT_SECONDS", "")
	t.Setenv("POSTGRES_CONNECT_TIMEOUT_SECONDS", "")
	t.Setenv("REDIS_DIAL_TIMEOUT_SECONDS", "")
	t.Setenv("REDIS_READ_TIMEOUT_SECONDS", "")
	t.Setenv("REDIS_WRITE_TIMEOUT_SECONDS", "")
	t.Setenv("MILVUS_CONNECT_TIMEOUT_SECONDS", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	content := []byte(`
postgres:
  connect_timeout_seconds: 9
redis:
  dial_timeout_seconds: 4
  read_timeout_seconds: 6
  write_timeout_seconds: 6
milvus:
  connect_timeout_seconds: 11
deepseek:
  timeout_seconds: 200
embedding:
  timeout_seconds: 80
rerank:
  timeout_seconds: 40
asr:
  timeout_seconds: 55
  max_audio_mb: 6
rag:
  query_expand_timeout_seconds: 12
  ocr:
    timeout_seconds: 50
memory:
  summary_timeout_seconds: 18
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.ConnectTimeoutSeconds != 9 {
		t.Fatalf("postgres=%d", cfg.Postgres.ConnectTimeoutSeconds)
	}
	if cfg.Redis.DialTimeoutSeconds != 4 || cfg.Redis.ReadTimeoutSeconds != 6 {
		t.Fatalf("redis dial=%d read=%d", cfg.Redis.DialTimeoutSeconds, cfg.Redis.ReadTimeoutSeconds)
	}
	if cfg.Milvus.ConnectTimeoutSeconds != 11 {
		t.Fatalf("milvus=%d", cfg.Milvus.ConnectTimeoutSeconds)
	}
	if cfg.DeepSeek.TimeoutSeconds != 200 {
		t.Fatalf("deepseek=%d", cfg.DeepSeek.TimeoutSeconds)
	}
	if cfg.Embedding.TimeoutSeconds != 80 {
		t.Fatalf("embedding=%d", cfg.Embedding.TimeoutSeconds)
	}
	if cfg.Rerank.TimeoutSeconds != 40 {
		t.Fatalf("rerank=%d", cfg.Rerank.TimeoutSeconds)
	}
	if cfg.ASR.TimeoutSeconds != 55 {
		t.Fatalf("asr=%d", cfg.ASR.TimeoutSeconds)
	}
	if cfg.ASR.MaxAudioMB != 6 {
		t.Fatalf("asr max=%d", cfg.ASR.MaxAudioMB)
	}
	if cfg.RAG.QueryExpandTimeoutSeconds != 12 {
		t.Fatalf("expand=%d", cfg.RAG.QueryExpandTimeoutSeconds)
	}
	if cfg.RAG.OCR.TimeoutSeconds != 50 {
		t.Fatalf("ocr=%d", cfg.RAG.OCR.TimeoutSeconds)
	}
	if cfg.Memory.SummaryTimeoutSeconds != 18 {
		t.Fatalf("summary=%d", cfg.Memory.SummaryTimeoutSeconds)
	}
}

func TestRedisWriteTimeoutFollowsRead(t *testing.T) {
	c := RedisConfig{ReadTimeoutSeconds: 8}
	if c.WriteTimeout() != 8*time.Second {
		t.Fatalf("write=%s", c.WriteTimeout())
	}
	c.WriteTimeoutSeconds = 3
	if c.WriteTimeout() != 3*time.Second {
		t.Fatalf("write override=%s", c.WriteTimeout())
	}
}

func TestASRFallbackToEmbedding(t *testing.T) {
	c := Config{
		Embedding: EmbeddingConfig{
			APIKey:  "emb-key",
			BaseURL: "https://api.siliconflow.cn/v1",
		},
	}
	c.setDefaults()
	if c.ASR.APIKey != "emb-key" {
		t.Fatalf("key=%s", c.ASR.APIKey)
	}
	if c.ASR.BaseURL != "https://api.siliconflow.cn/v1" {
		t.Fatalf("url=%s", c.ASR.BaseURL)
	}
	if c.ASR.Language != "zh" {
		t.Fatalf("lang=%s", c.ASR.Language)
	}
	if c.TTS.APIKey != "emb-key" {
		t.Fatalf("tts key=%s", c.TTS.APIKey)
	}
	if c.TTS.Model != "FunAudioLLM/CosyVoice2-0.5B" {
		t.Fatalf("tts model=%s", c.TTS.Model)
	}
	if c.TTS.Voice != "anna" {
		t.Fatalf("tts voice=%s", c.TTS.Voice)
	}
	if c.TTS.MaxChars != 4000 {
		t.Fatalf("tts max=%d", c.TTS.MaxChars)
	}
}
