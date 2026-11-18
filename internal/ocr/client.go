package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

var (
	// ErrNotConfigured API Key / BaseURL 缺失
	ErrNotConfigured = errors.New("OCR 未配置：请设置 rag.ocr.api_key 或 embedding.api_key")
	// ErrEmptyFile 文件为空
	ErrEmptyFile = errors.New("OCR 文件内容为空")
)

var (
	reRefTag = regexp.MustCompile(`(?s)<\|ref\|>.*?<\|/ref\|>`)
	reDetTag = regexp.MustCompile(`(?s)<\|det\|>.*?<\|/det\|>`)
	reMiscTag = regexp.MustCompile(`<\|[^|>]+\|>`)
)

// Client OpenAI 兼容 /chat/completions 视觉 OCR（如 SiliconFlow DeepSeek-OCR）
type Client struct {
	cfg  config.OCRConfig
	http *http.Client
}

// NewClient 创建 OCR 客户端。
func NewClient(cfg config.OCRConfig) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout(),
		},
	}
}

// Configured 是否具备调用上游所需的凭据。
func (c *Client) Configured() bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.cfg.APIKey) != "" && strings.TrimSpace(c.cfg.BaseURL) != ""
}

// UpstreamError 上游 OCR HTTP 错误
type UpstreamError struct {
	Status int
	Body   string
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "ocr upstream error"
	}
	return fmt.Sprintf("ocr http %d: %s", e.Status, e.Body)
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Message string `json:"message"`
}

// Recognize 实现图片 OCR（parser.ImageRecognizer）。
func (c *Client) Recognize(ctx context.Context, imagePath string) (string, error) {
	return c.recognizeFile(ctx, imagePath, "")
}

// RecognizePDF 整份 PDF 一次识别；pages 仅用于决定结果挂到哪一页（取第一页）。
func (c *Client) RecognizePDF(ctx context.Context, pdfPath string, pages []int) (map[int]string, error) {
	if len(pages) == 0 {
		return map[int]string{}, nil
	}
	text, err := c.recognizeFile(ctx, pdfPath, "application/pdf")
	out := make(map[int]string, 1)
	if strings.TrimSpace(text) != "" {
		out[pages[0]] = strings.TrimSpace(text)
	}
	return out, err
}

func (c *Client) recognizeFile(ctx context.Context, path, mimeHint string) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", ErrEmptyFile
	}

	mime := strings.TrimSpace(mimeHint)
	if mime == "" {
		mime = detectMIME(path, data)
	}
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	prompt := strings.TrimSpace(c.cfg.Prompt)
	if prompt == "" {
		prompt = "Free OCR."
	}
	maxTokens := c.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	payload := chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{{
			Role: "user",
			Content: []contentPart{
				{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
				{Type: "text", Text: prompt},
			},
		}},
		MaxTokens:   maxTokens,
		Temperature: 0,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("OCR 服务超时（%s）", c.cfg.BaseURL)
		}
		return "", fmt.Errorf("OCR 服务不可达（%s）: %w", c.cfg.BaseURL, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("read ocr response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &UpstreamError{Status: resp.StatusCode, Body: clip(string(raw), 300)}
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("OCR 服务响应无效: %w", err)
	}
	if len(parsed.Choices) == 0 {
		if msg := strings.TrimSpace(parsed.Message); msg != "" {
			return "", fmt.Errorf("OCR 服务: %s", msg)
		}
		return "", fmt.Errorf("OCR 服务未返回内容")
	}
	return cleanOCRText(parsed.Choices[0].Message.Content), nil
}

func detectMIME(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".pdf":
		return "application/pdf"
	}
	if len(data) >= 4 && string(data[:4]) == "%PDF" {
		return "application/pdf"
	}
	ct := http.DetectContentType(data)
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	return "application/octet-stream"
}

func cleanOCRText(s string) string {
	s = reRefTag.ReplaceAllString(s, "")
	s = reDetTag.ReplaceAllString(s, "")
	s = reMiscTag.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
