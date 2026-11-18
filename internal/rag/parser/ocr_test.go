package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
	"github.com/kelvins-io/eino-repository-rag/internal/ocr"
)

// 1x1 PNG（无文字，仅作容器）
var png1x1 = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestDetectImageFormat(t *testing.T) {
	if got := detectFormat("x.bin", "image/png"); got != "image" {
		t.Fatalf("content-type image got %s", got)
	}
}

func TestExtractImageRequiresOCR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scan.png")
	if err := os.WriteFile(path, png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ExtractFile(path, "image/png")
	if err == nil || !IsPermanent(err) {
		t.Fatalf("expected permanent OCR-required error, got %v", err)
	}
	if !strings.Contains(err.Error(), "OCR") {
		t.Fatalf("error should mention OCR: %v", err)
	}
}

func TestExtractImageWithModelOCR(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"content": "HTTP扫描正文"},
			}},
		})
	}))
	t.Cleanup(ts.Close)

	dir := t.TempDir()
	path := filepath.Join(dir, "scan.png")
	if err := os.WriteFile(path, png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ExtractFile(path, "", WithOCR(OCR{
		Enabled: true,
		Config: config.OCRConfig{
			APIKey:  "test-key",
			BaseURL: ts.URL,
			Model:   "deepseek-ai/DeepSeek-OCR",
			Prompt:  "Free OCR.",
		},
		Client: ocr.NewClient(config.OCRConfig{
			APIKey:         "test-key",
			BaseURL:        ts.URL,
			Model:          "deepseek-ai/DeepSeek-OCR",
			Prompt:         "Free OCR.",
			TimeoutSeconds: 5,
			MaxTokens:      256,
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "HTTP扫描正文") {
		t.Fatalf("text=%q", res.Text)
	}
}

func TestExtractImageWithStubOCR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scan.png")
	if err := os.WriteFile(path, png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ExtractFile(path, "", WithOCR(OCR{
		Enabled: true,
		Recognizer: ImageRecognizerFunc(func(ctx context.Context, imagePath string) (string, error) {
			if imagePath == "" {
				return "", fmt.Errorf("empty path")
			}
			return "扫描合同正文", nil
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "image" {
		t.Fatalf("format=%s", res.Format)
	}
	if !strings.Contains(res.Text, "扫描合同正文") {
		t.Fatalf("text=%q", res.Text)
	}
}

func TestExtractImageOnlyPDFWithStubOCR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(path, imageOnlyPDF(), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ExtractFile(path, "application/pdf")
	if err == nil || !IsPermanent(err) {
		t.Fatalf("expected permanent error without OCR, got %v", err)
	}

	res, err := ExtractFile(path, "application/pdf", WithOCR(OCR{
		Enabled: true,
		Recognizer: ImageRecognizerFunc(func(ctx context.Context, imagePath string) (string, error) {
			return "第1页扫描内容", nil
		}),
		RenderPDFPage: func(ctx context.Context, pdfPath string, page, dpi int, outDir string) (string, error) {
			p := filepath.Join(outDir, fmt.Sprintf("p%d.png", page))
			if err := os.WriteFile(p, png1x1, 0o644); err != nil {
				return "", err
			}
			return p, nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "第1页扫描内容") {
		t.Fatalf("ocr text=%q", res.Text)
	}
	if !strings.Contains(res.Text, "第 1 页") {
		t.Fatalf("missing page marker: %q", res.Text)
	}
}

func TestExtractPDFWithWholeFileOCR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scan.pdf")
	if err := os.WriteFile(path, imageOnlyPDF(), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ExtractFile(path, "application/pdf", WithOCR(OCR{
		Enabled: true,
		PDF: pdfRecognizerFunc(func(ctx context.Context, pdfPath string, pages []int) (map[int]string, error) {
			if len(pages) == 0 {
				return nil, fmt.Errorf("no pages")
			}
			return map[int]string{pages[0]: "整份PDF识别结果"}, nil
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "整份PDF识别结果") {
		t.Fatalf("text=%q", res.Text)
	}
}

type pdfRecognizerFunc func(ctx context.Context, pdfPath string, pages []int) (map[int]string, error)

func (f pdfRecognizerFunc) RecognizePDF(ctx context.Context, pdfPath string, pages []int) (map[int]string, error) {
	return f(ctx, pdfPath, pages)
}

func imageOnlyPDF() []byte {
	pageContent := "/Img0 Do"
	return buildMinimalPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Img0 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /Filter /DCTDecode /Length 0 >>\nstream\nendstream",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(pageContent), pageContent),
	})
}

func buildMinimalPDF(objs []string) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	off := make([]int, len(objs)+1)
	for i, body := range objs {
		off[i+1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xrefOff := b.Len()
	n := len(objs) + 1
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", n)
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", off[i])
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", n, xrefOff)
	return []byte(b.String())
}
