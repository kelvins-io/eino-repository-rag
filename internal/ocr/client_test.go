package ocr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

func TestRecognizeImageChatCompletions(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing bearer")
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{
					"content": "<|ref|>x<|/ref|>识别结果<|det|>1<|/det|>",
				},
			}},
		})
	}))
	t.Cleanup(ts.Close)

	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	if err := os.WriteFile(path, []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewClient(config.OCRConfig{
		APIKey:         "k",
		BaseURL:        ts.URL,
		Model:          "deepseek-ai/DeepSeek-OCR",
		Prompt:         "Free OCR.",
		TimeoutSeconds: 5,
		MaxTokens:      128,
	})
	text, err := c.Recognize(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if text != "识别结果" {
		t.Fatalf("text=%q", text)
	}
	if gotBody["model"] != "deepseek-ai/DeepSeek-OCR" {
		t.Fatalf("model=%v", gotBody["model"])
	}
}

func TestRecognizePDFWholeFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Content []struct {
					Type     string `json:"type"`
					ImageURL *struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(raw, &req)
		if len(req.Messages) == 0 || len(req.Messages[0].Content) == 0 {
			t.Fatalf("empty messages")
		}
		url := ""
		for _, p := range req.Messages[0].Content {
			if p.Type == "image_url" && p.ImageURL != nil {
				url = p.ImageURL.URL
			}
		}
		if !strings.HasPrefix(url, "data:application/pdf;base64,") {
			t.Fatalf("data url=%q", url)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"content": "PDF正文"},
			}},
		})
	}))
	t.Cleanup(ts.Close)

	dir := t.TempDir()
	path := filepath.Join(dir, "a.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4 fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewClient(config.OCRConfig{
		APIKey:         "k",
		BaseURL:        ts.URL,
		Model:          "deepseek-ai/DeepSeek-OCR",
		TimeoutSeconds: 5,
		MaxTokens:      128,
	})
	out, err := c.RecognizePDF(context.Background(), path, []int{2, 5})
	if err != nil {
		t.Fatal(err)
	}
	if out[2] != "PDF正文" {
		t.Fatalf("out=%v", out)
	}
	if _, ok := out[5]; ok {
		t.Fatalf("should only set first page key")
	}
}

func TestNotConfigured(t *testing.T) {
	c := NewClient(config.OCRConfig{})
	if c.Configured() {
		t.Fatal("expected not configured")
	}
	_, err := c.Recognize(context.Background(), "x.png")
	if err != ErrNotConfigured {
		t.Fatalf("err=%v", err)
	}
}
