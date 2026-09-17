package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

var (
	// ErrNotConfigured API Key / BaseURL 缺失
	ErrNotConfigured = errors.New("语音识别未配置：请设置 asr.api_key 或 embedding.api_key")
	// ErrEmptyAudio 上传音频长度为 0
	ErrEmptyAudio = errors.New("音频内容为空")
)

// Transcriber 语音转写
type Transcriber interface {
	Transcribe(ctx context.Context, req Request) (string, error)
}

// Request 一次转写请求
type Request struct {
	Filename    string
	ContentType string
	Body        io.Reader
	Language    string
	Prompt      string
}

// Client OpenAI 兼容 /audio/transcriptions（SiliconFlow SenseVoice / Whisper 等）
type Client struct {
	cfg  config.ASRConfig
	http *http.Client
}

func NewClient(cfg config.ASRConfig) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout(),
		},
	}
}

// UpstreamError 上游 ASR HTTP 错误
type UpstreamError struct {
	Status int
	Body   string
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "asr upstream error"
	}
	return fmt.Sprintf("asr http %d: %s", e.Status, e.Body)
}

type transcriptionResponse struct {
	Text    string `json:"text"`
	Message string `json:"message"`
}

func (c *Client) Transcribe(ctx context.Context, req Request) (string, error) {
	if c == nil {
		return "", ErrNotConfigured
	}
	if strings.TrimSpace(c.cfg.APIKey) == "" || strings.TrimSpace(c.cfg.BaseURL) == "" {
		return "", ErrNotConfigured
	}
	if req.Body == nil {
		return "", ErrEmptyAudio
	}

	filename := sanitizeFilename(req.Filename)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreatePart(filePartHeader(filename, req.ContentType))
	if err != nil {
		return "", err
	}
	n, err := io.Copy(part, req.Body)
	if err != nil {
		return "", fmt.Errorf("read audio: %w", err)
	}
	if n == 0 {
		return "", ErrEmptyAudio
	}
	if err := w.WriteField("model", c.cfg.Model); err != nil {
		return "", err
	}
	lang := strings.TrimSpace(req.Language)
	if lang == "" {
		lang = strings.TrimSpace(c.cfg.Language)
	}
	if lang != "" {
		if err := w.WriteField("language", lang); err != nil {
			return "", err
		}
	}
	if prompt := joinPrompt(c.cfg.Prompt, req.Prompt); prompt != "" {
		if err := w.WriteField("prompt", prompt); err != nil {
			return "", err
		}
	}
	if err := w.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	url := strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/") + "/audio/transcriptions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("asr request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read asr response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &UpstreamError{Status: resp.StatusCode, Body: truncate(string(raw), 300)}
	}

	var parsed transcriptionResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode asr response: %w", err)
	}
	text := strings.TrimSpace(parsed.Text)
	return text, nil
}

func joinPrompt(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if r == 0 || r == '/' || r == '\\' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return "speech.webm"
	}
	return name
}

func filePartHeader(filename, contentType string) textproto.MIMEHeader {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	return h
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
