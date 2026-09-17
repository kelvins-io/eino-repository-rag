package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Client 调用 compose OCR sidecar。
type Client struct {
	endpoint  string
	http      *http.Client
	languages string
	psm       int
	timeout   time.Duration
}

// NewClient endpoint 形如 http://localhost:18080。
func NewClient(endpoint string, timeout time.Duration, languages string, psm int) *Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if strings.TrimSpace(languages) == "" {
		languages = "chi_sim+eng"
	}
	if psm <= 0 {
		psm = 6
	}
	return &Client{
		endpoint:  strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		http:      &http.Client{},
		languages: languages,
		psm:       psm,
		timeout:   timeout,
	}
}

// Recognize 实现图片 OCR（parser.ImageRecognizer）。
func (c *Client) Recognize(ctx context.Context, imagePath string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var resp imageResponse
	if err := c.postFile(ctx, "/v1/ocr/image", imagePath, map[string]string{
		"languages": c.languages,
		"psm":       strconv.Itoa(c.psm),
	}, &resp); err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}

// RecognizePDF 上传 PDF 并识别指定页。
func (c *Client) RecognizePDF(ctx context.Context, pdfPath string, pages []int, dpi int) (map[int]string, error) {
	if len(pages) == 0 {
		return map[int]string{}, nil
	}
	if dpi <= 0 {
		dpi = 200
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// 整份 PDF 一次上传；超时按页数放宽，仍受外部 ctx 约束
	per := c.timeout
	if per <= 0 {
		per = 60 * time.Second
	}
	limit := per * time.Duration(len(pages))
	if limit < per {
		limit = per
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	parts := make([]string, 0, len(pages))
	for _, p := range pages {
		parts = append(parts, strconv.Itoa(p))
	}
	var resp pdfResponse
	if err := c.postFile(ctx, "/v1/ocr/pdf", pdfPath, map[string]string{
		"pages":     strings.Join(parts, ","),
		"languages": c.languages,
		"psm":       strconv.Itoa(c.psm),
		"dpi":       strconv.Itoa(dpi),
	}, &resp); err != nil {
		return nil, err
	}
	out := make(map[int]string, len(resp.Pages))
	var firstErr error
	for _, p := range resp.Pages {
		if strings.TrimSpace(p.Text) != "" {
			out[p.Page] = strings.TrimSpace(p.Text)
			continue
		}
		if p.Error != "" && firstErr == nil {
			firstErr = fmt.Errorf("第 %d 页: %s", p.Page, p.Error)
		}
	}
	if len(out) == 0 && firstErr != nil {
		return out, firstErr
	}
	return out, firstErr
}

func (c *Client) postFile(ctx context.Context, path, filePath string, fields map[string]string, dest any) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return err
		}
	}
	if err := mw.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("OCR 服务超时（%s）", c.endpoint)
		}
		return fmt.Errorf("OCR 服务不可达（%s）。请先执行：docker compose up -d ocr", c.endpoint)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		var er errorResponse
		if json.Unmarshal(raw, &er) == nil && er.Error != "" {
			return fmt.Errorf("OCR 服务: %s", er.Error)
		}
		return fmt.Errorf("OCR 服务 HTTP %d: %s", resp.StatusCode, clip(string(raw), 240))
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("OCR 服务响应无效: %w", err)
	}
	return nil
}
