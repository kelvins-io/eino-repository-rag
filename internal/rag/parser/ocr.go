package parser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/ocr"
)

// ImageRecognizer 将图片识别为纯文本（测试可注入）。
type ImageRecognizer interface {
	Recognize(ctx context.Context, imagePath string) (string, error)
}

// ImageRecognizerFunc 适配函数为 ImageRecognizer。
type ImageRecognizerFunc func(ctx context.Context, imagePath string) (string, error)

func (f ImageRecognizerFunc) Recognize(ctx context.Context, imagePath string) (string, error) {
	return f(ctx, imagePath)
}

// OCR 扫描件 / 图片 OCR 配置。
type OCR struct {
	Enabled      bool
	Endpoint     string // 非空则调用 compose OCR 服务，如 http://localhost:18080
	Languages    string
	DPI          int
	Concurrency  int
	Timeout      time.Duration
	PageSegMode  int
	TesseractBin string
	PDFToPPMBin  string

	// Recognizer / RenderPDFPage 仅测试注入。
	Recognizer    ImageRecognizer
	RenderPDFPage func(ctx context.Context, pdfPath string, page, dpi int, dir string) (string, error)
}

type extractSettings struct {
	ctx context.Context
	ocr OCR
}

// ExtractOption 控制 ExtractFile 行为。
type ExtractOption func(*extractSettings)

// WithContext 绑定取消/超时（OCR 渲染与识别会遵守）。
func WithContext(ctx context.Context) ExtractOption {
	return func(s *extractSettings) {
		if ctx != nil {
			s.ctx = ctx
		}
	}
}

// WithOCR 启用扫描页/图片 OCR。
func WithOCR(ocr OCR) ExtractOption {
	return func(s *extractSettings) {
		s.ocr = ocr
	}
}

func defaultExtractSettings() extractSettings {
	return extractSettings{ctx: context.Background()}
}

func applyExtractOptions(opts []ExtractOption) extractSettings {
	s := defaultExtractSettings()
	for _, opt := range opts {
		if opt != nil {
			opt(&s)
		}
	}
	if s.ctx == nil {
		s.ctx = context.Background()
	}
	return s
}

func (s *extractSettings) ocrEnabled() bool {
	return s != nil && s.ocr.Enabled
}

func (s *extractSettings) ocrConfig() ocr.Config {
	return ocr.Config{
		Languages:    s.ocr.Languages,
		DPI:          s.ocr.DPI,
		Concurrency:  s.ocr.Concurrency,
		Timeout:      s.ocr.Timeout,
		PageSegMode:  s.ocr.PageSegMode,
		TesseractBin: s.ocr.TesseractBin,
		PDFToPPMBin:  s.ocr.PDFToPPMBin,
	}
}

func (s *extractSettings) httpOCR() *ocr.Client {
	ep := strings.TrimSpace(s.ocr.Endpoint)
	if ep == "" {
		return nil
	}
	return ocr.NewClient(ep, s.ocr.Timeout, s.ocr.Languages, s.ocr.PageSegMode)
}

func (s *extractSettings) recognizer() (ImageRecognizer, error) {
	if s == nil || !s.ocr.Enabled {
		return nil, nil
	}
	if s.ocr.Recognizer != nil {
		return s.ocr.Recognizer, nil
	}
	if c := s.httpOCR(); c != nil {
		s.ocr.Recognizer = c
		return c, nil
	}
	eng := &localRecognizer{cfg: s.ocrConfig()}
	s.ocr.Recognizer = eng
	return eng, nil
}

type localRecognizer struct {
	cfg ocr.Config
}

func (l *localRecognizer) Recognize(ctx context.Context, imagePath string) (string, error) {
	text, err := ocr.RecognizeImage(ctx, l.cfg, imagePath)
	if err != nil {
		return "", err
	}
	return sanitizeExtractedText(text), nil
}

func ocrImageFile(ctx context.Context, s *extractSettings, path string) (string, error) {
	rec, err := s.recognizer()
	if err != nil {
		return "", err
	}
	if rec == nil {
		return "", nil
	}
	text, err := rec.Recognize(ctx, path)
	if err != nil {
		return "", err
	}
	return normalizeText(text), nil
}

func ocrImageBytes(ctx context.Context, s *extractSettings, dir, name string, data []byte) (string, error) {
	rec, err := s.recognizer()
	if err != nil {
		return "", err
	}
	if rec == nil {
		return "", nil
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" || ext == ".bin" {
		ext = ".png"
	}
	tmp := filepath.Join(dir, "img-"+strconv.FormatInt(time.Now().UnixNano(), 10)+ext)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	defer os.Remove(tmp)
	text, err := rec.Recognize(ctx, tmp)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sanitizeExtractedText(text)), nil
}

func ocrPDFPages(ctx context.Context, pdfPath string, pages []pdfPageWork, s *extractSettings) error {
	need := make([]int, 0)
	for i := range pages {
		if pages[i].needOCR {
			need = append(need, pages[i].num)
		}
	}
	if len(need) == 0 || !s.ocrEnabled() {
		return nil
	}

	dpi := s.ocr.DPI
	if dpi <= 0 {
		dpi = 200
	}

	var (
		out map[int]string
		err error
	)
	if s.ocr.RenderPDFPage != nil {
		out, err = ocrPDFPagesInjected(ctx, pdfPath, need, s, dpi)
	} else if c := s.httpOCR(); c != nil {
		out, err = c.RecognizePDF(ctx, pdfPath, need, dpi)
	} else {
		out, err = ocr.RecognizePDFPages(ctx, s.ocrConfig(), pdfPath, need)
	}
	if out == nil {
		out = map[int]string{}
	}

	for i := range pages {
		if text := strings.TrimSpace(sanitizeExtractedText(out[pages[i].num])); text != "" {
			pages[i].text = text
			pages[i].ocrOK = true
		}
	}
	return err
}

func ocrPDFPagesInjected(ctx context.Context, pdfPath string, need []int, s *extractSettings, dpi int) (map[int]string, error) {
	rec, err := s.recognizer()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "rag-ocr-pdf-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	conc := s.ocr.Concurrency
	if conc <= 0 {
		conc = 2
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	var mu sync.Mutex
	out := make(map[int]string, len(need))
	var firstErr error

	for _, page := range need {
		page := page
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				mu.Lock()
				if firstErr == nil {
					firstErr = ctx.Err()
				}
				mu.Unlock()
				return
			case sem <- struct{}{}:
				defer func() { <-sem }()
			}
			img, err := s.ocr.RenderPDFPage(ctx, pdfPath, page, dpi, dir)
			if err == nil {
				var text string
				text, err = rec.Recognize(ctx, img)
				text = strings.TrimSpace(sanitizeExtractedText(text))
				if err == nil && text != "" {
					mu.Lock()
					out[page] = text
					mu.Unlock()
					return
				}
			}
			mu.Lock()
			if firstErr == nil && err != nil {
				firstErr = fmt.Errorf("第 %d 页: %w", page, err)
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out, firstErr
}

func truncateRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}
