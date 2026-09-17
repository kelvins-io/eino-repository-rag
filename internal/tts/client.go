package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

var (
	// ErrNotConfigured API Key / BaseURL 缺失
	ErrNotConfigured = errors.New("语音合成未配置：请设置 tts.api_key 或 embedding.api_key")
	// ErrEmptyText 朗读文本为空
	ErrEmptyText = errors.New("没有可朗读的内容")
	citationRe   = regexp.MustCompile(`\[\d+]`)
	spaceRe      = regexp.MustCompile(`[ \t\x00-\x08\x0b\x0c\x0e-\x1f]+`)
)

const maxAudioBytes = 8 << 20

// Speaker 文本转语音
type Speaker interface {
	Synthesize(ctx context.Context, text string) (Result, error)
}

// Result 合成音频
type Result struct {
	Audio       []byte
	ContentType string
}

// Client OpenAI 兼容 /audio/speech（SiliconFlow CosyVoice 等）
type Client struct {
	cfg  config.TTSConfig
	http *http.Client
}

func NewClient(cfg config.TTSConfig) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout(),
		},
	}
}

// UpstreamError 上游 TTS HTTP 错误
type UpstreamError struct {
	Status int
	Body   string
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "tts upstream error"
	}
	return fmt.Sprintf("tts http %d: %s", e.Status, e.Body)
}

type speechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice,omitempty"`
	ResponseFormat string  `json:"response_format"`
	Speed          float32 `json:"speed,omitempty"`
}

func (c *Client) Synthesize(ctx context.Context, text string) (Result, error) {
	if c == nil {
		return Result{}, ErrNotConfigured
	}
	if strings.TrimSpace(c.cfg.APIKey) == "" || strings.TrimSpace(c.cfg.BaseURL) == "" {
		return Result{}, ErrNotConfigured
	}
	input := PrepareText(text, c.cfg.MaxChars)
	if input == "" {
		return Result{}, ErrEmptyText
	}

	body, err := json.Marshal(speechRequest{
		Model:          c.cfg.Model,
		Input:          input,
		Voice:          resolveVoice(c.cfg.Model, c.cfg.Voice),
		ResponseFormat: "mp3",
		Speed:          1,
	})
	if err != nil {
		return Result{}, err
	}

	url := strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/") + "/audio/speech"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("tts request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAudioBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("read tts response: %w", err)
	}
	if int64(len(raw)) > maxAudioBytes {
		return Result{}, fmt.Errorf("tts audio too large")
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || strings.Contains(ct, "json") {
		return Result{}, &UpstreamError{Status: resp.StatusCode, Body: truncate(string(raw), 300)}
	}
	if len(raw) == 0 {
		return Result{}, &UpstreamError{Status: resp.StatusCode, Body: "empty audio"}
	}
	if ct == "" || strings.Contains(ct, "octet-stream") || strings.Contains(ct, "application/audio") {
		ct = "audio/mpeg"
	}
	return Result{Audio: raw, ContentType: ct}, nil
}

// PrepareText 去掉引用标记并截断，供朗读使用
func PrepareText(text string, maxChars int) string {
	s := strings.ReplaceAll(text, "\r\n", "\n")
	s = citationRe.ReplaceAllString(s, "")
	s = spaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if maxChars <= 0 {
		maxChars = 4000
	}
	if utf8.RuneCountInString(s) <= maxChars {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:maxChars]))
}

func resolveVoice(model, voice string) string {
	voice = strings.TrimSpace(voice)
	model = strings.TrimSpace(model)
	if voice == "" {
		if model == "" {
			return "anna"
		}
		return model + ":anna"
	}
	if strings.Contains(voice, ":") || model == "" {
		return voice
	}
	return model + ":" + voice
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
