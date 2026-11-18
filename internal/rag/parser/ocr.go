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

	"github.com/kelvins-io/eino-repository-rag/internal/config"
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

// PDFRecognizer 整份 PDF OCR（测试可注入）。
type PDFRecognizer interface {
	RecognizePDF(ctx context.Context, pdfPath string, pages []int) (map[int]string, error)
}

// OCR 扫描件 / 图片 OCR 配置。
type OCR struct {
	Enabled bool
	Config  config.OCRConfig

	// Client 非空时优先使用；否则按 Config 构造。
	Client *ocr.Client

	// Recognizer / PDF / RenderPDFPage 仅测试注入。
	Recognizer    ImageRecognizer
	PDF           PDFRecognizer
	RenderPDFPage func(ctx context.Context, pdfPath string, page, dpi int, dir string) (string, error)
	// Concurrency 仅注入 RenderPDFPage 路径使用。
	Concurrency int
	DPI         int
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

func (s *extractSettings) modelOCR() *ocr.Client {
	if s == nil {
		return nil
	}
	if s.ocr.Client != nil {
		return s.ocr.Client
	}
	if !s.ocr.Enabled {
		return nil
	}
	c := ocr.NewClient(s.ocr.Config)
	if !c.Configured() {
		return nil
	}
	s.ocr.Client = c
	return c
}

func (s *extractSettings) recognizer() (ImageRecognizer, error) {
	if s == nil || !s.ocr.Enabled {
		return nil, nil
	}
	if s.ocr.Recognizer != nil {
		return s.ocr.Recognizer, nil
	}
	if c := s.modelOCR(); c != nil {
		s.ocr.Recognizer = c
		return c, nil
	}
	return nil, fmt.Errorf("%w", ocr.ErrNotConfigured)
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
	switch {
	case s.ocr.RenderPDFPage != nil:
		out, err = ocrPDFPagesInjected(ctx, pdfPath, need, s, dpi)
	case s.ocr.PDF != nil:
		out, err = s.ocr.PDF.RecognizePDF(ctx, pdfPath, need)
	default:
		c := s.modelOCR()
		if c == nil {
			return fmt.Errorf("%w", ocr.ErrNotConfigured)
		}
		out, err = c.RecognizePDF(ctx, pdfPath, need)
	}
	if out == nil {
		out = map[int]string{}
	}

	// 整份 PDF OCR：结果只挂在第一个 need 页；其余 need 页标记已处理避免重复拼接。
	firstNeed := need[0]
	full := strings.TrimSpace(sanitizeExtractedText(out[firstNeed]))
	if full == "" {
		// 兼容注入路径按页返回
		for _, p := range need {
			if t := strings.TrimSpace(sanitizeExtractedText(out[p])); t != "" {
				full = t
				break
			}
		}
	}

	assigned := false
	for i := range pages {
		if !pages[i].needOCR {
			continue
		}
		if !assigned && full != "" {
			pages[i].text = full
			pages[i].ocrOK = true
			assigned = true
			continue
		}
		if text := strings.TrimSpace(sanitizeExtractedText(out[pages[i].num])); text != "" && s.ocr.RenderPDFPage != nil {
			pages[i].text = text
			pages[i].ocrOK = true
			continue
		}
		// 整份识别已写入首个 need 页；其余页不再重复写入
		if assigned && s.ocr.RenderPDFPage == nil {
			pages[i].ocrOK = true
			pages[i].text = ""
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
