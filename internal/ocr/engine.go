package ocr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/logger"
)

// Config 本地 tesseract / pdftoppm 执行参数。
type Config struct {
	Languages    string
	DPI          int
	Concurrency  int
	Timeout      time.Duration
	PageSegMode  int
	TesseractBin string
	PDFToPPMBin  string
}

func (c Config) langs() string {
	if strings.TrimSpace(c.Languages) == "" {
		return "chi_sim+eng"
	}
	return strings.TrimSpace(c.Languages)
}

func (c Config) psm() int {
	if c.PageSegMode <= 0 {
		return 6
	}
	return c.PageSegMode
}

func (c Config) dpi() int {
	if c.DPI <= 0 {
		return 200
	}
	return c.DPI
}

func (c Config) conc() int {
	if c.Concurrency <= 0 {
		return 1
	}
	return c.Concurrency
}

var (
	slotOnce sync.Once
	slots    chan struct{}
)

func maxProcs() int {
	n := 1
	if v := strings.TrimSpace(os.Getenv("OCR_MAX_PROCS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}
	return n
}

func acquireSlot(ctx context.Context) error {
	slotOnce.Do(func() {
		n := maxProcs()
		slots = make(chan struct{}, n)
		logger.S().Infof("[ocr] worker slots=%d", n)
	})
	wait := time.Now()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case slots <- struct{}{}:
		if d := time.Since(wait); d >= time.Second {
			logger.S().Warnf("[%s] wait slot %s", jobOf(ctx), d.Round(time.Millisecond))
		}
		return nil
	}
}

func releaseSlot() {
	<-slots
}

func tesseractEnv() []string {
	env := os.Environ()
	// Tesseract/Leptonica 默认 OpenMP 打满所有核；多进程叠加会到 700%+
	env = append(env, "OMP_THREAD_LIMIT=1", "OMP_NUM_THREADS=1")
	return env
}

func (c Config) timeout() time.Duration {
	if c.Timeout <= 0 {
		return 60 * time.Second
	}
	return c.Timeout
}

func (c Config) tesseract() (string, error) {
	bin := strings.TrimSpace(c.TesseractBin)
	if bin == "" {
		bin = "tesseract"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("未找到 tesseract（%s）。请启动 OCR 服务：docker compose up -d ocr", bin)
	}
	return path, nil
}

func (c Config) pdftoppm() (string, error) {
	bin := strings.TrimSpace(c.PDFToPPMBin)
	if bin == "" {
		bin = "pdftoppm"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("未找到 pdftoppm（%s）。请启动 OCR 服务：docker compose up -d ocr", bin)
	}
	return path, nil
}

// RecognizeImage 对本地图片跑 tesseract。
func RecognizeImage(ctx context.Context, cfg Config, imagePath string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := acquireSlot(ctx); err != nil {
		return "", err
	}
	defer releaseSlot()
	return recognizeImage(ctx, cfg, imagePath)
}

func recognizeImage(ctx context.Context, cfg Config, imagePath string) (string, error) {
	bin, err := cfg.tesseract()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout())
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, imagePath, "stdout", "-l", cfg.langs(), "--psm", strconv.Itoa(cfg.psm()))
	cmd.Env = tesseractEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if ctx.Err() != nil {
			return "", fmt.Errorf("tesseract 超时: %w", ctx.Err())
		}
		if msg != "" {
			return "", fmt.Errorf("tesseract: %w (%s)", err, clip(msg, 240))
		}
		return "", fmt.Errorf("tesseract: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RenderPDFPage 用 pdftoppm 把 PDF 单页渲成 PNG，返回图片路径。
func RenderPDFPage(ctx context.Context, cfg Config, pdfPath string, page int, dir string) (string, error) {
	bin, err := cfg.pdftoppm()
	if err != nil {
		return "", err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prefix := filepath.Join(dir, fmt.Sprintf("p%d", page))
	cmd := exec.CommandContext(ctx, bin,
		"-png",
		"-r", strconv.Itoa(cfg.dpi()),
		"-f", strconv.Itoa(page),
		"-l", strconv.Itoa(page),
		"-singlefile",
		pdfPath,
		prefix,
	)
	cmd.Env = tesseractEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if ctx.Err() != nil {
			return "", fmt.Errorf("pdftoppm 第 %d 页: %w", page, ctx.Err())
		}
		if msg != "" {
			return "", fmt.Errorf("pdftoppm 第 %d 页: %w (%s)", page, err, clip(msg, 240))
		}
		return "", fmt.Errorf("pdftoppm 第 %d 页: %w", page, err)
	}
	png := prefix + ".png"
	if _, statErr := os.Stat(png); statErr != nil {
		return "", fmt.Errorf("pdftoppm 第 %d 页未生成 %s", page, png)
	}
	return png, nil
}

// RecognizePDFPages 渲染并识别指定页，返回 page->text。
func RecognizePDFPages(ctx context.Context, cfg Config, pdfPath string, pages []int) (map[int]string, error) {
	if len(pages) == 0 {
		return map[int]string{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dir, err := os.MkdirTemp("", "rag-ocr-pdf-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	sem := make(chan struct{}, cfg.conc())
	var wg sync.WaitGroup
	var mu sync.Mutex
	var done atomic.Int32
	total := len(pages)
	out := make(map[int]string, total)
	var firstErr error

	for _, page := range pages {
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
			if err := acquireSlot(ctx); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			func() {
				defer releaseSlot()
				job := jobOf(ctx)
				logger.S().Infof("[%s] page start p=%d dpi=%d", job, page, cfg.dpi())
				t0 := time.Now()
				img, err := RenderPDFPage(ctx, cfg, pdfPath, page, dir)
				renderDur := time.Since(t0)
				var text string
				ocrDur := time.Duration(0)
				if err == nil {
					t1 := time.Now()
					text, err = recognizeImage(ctx, cfg, img)
					ocrDur = time.Since(t1)
				}
				n := int(done.Add(1))
				if err != nil {
					logger.S().Errorf("[%s] page %d/%d p=%d fail=%v render=%s ocr=%s",
						job, n, total, page, err, renderDur.Round(time.Millisecond), ocrDur.Round(time.Millisecond))
				} else {
					logger.S().Infof("[%s] page %d/%d p=%d runes=%d render=%s ocr=%s",
						job, n, total, page, len([]rune(text)), renderDur.Round(time.Millisecond), ocrDur.Round(time.Millisecond))
				}
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("第 %d 页: %w", page, err)
					}
					return
				}
				if text != "" {
					out[page] = text
				}
			}()
		}()
	}
	wg.Wait()
	return out, firstErr
}

func clip(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}

type jobCtxKey struct{}

var jobSeq atomic.Int64

func nextJobID(kind string) string {
	return fmt.Sprintf("%s#%d", kind, jobSeq.Add(1))
}

func withJob(ctx context.Context, job string) context.Context {
	return context.WithValue(ctx, jobCtxKey{}, job)
}

func jobOf(ctx context.Context) string {
	if ctx == nil {
		return "ocr"
	}
	if v, ok := ctx.Value(jobCtxKey{}).(string); ok && v != "" {
		return v
	}
	return "ocr"
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func pageRange(pages []int) string {
	if len(pages) == 0 {
		return "-"
	}
	if len(pages) == 1 {
		return strconv.Itoa(pages[0])
	}
	ok := true
	for i := 1; i < len(pages); i++ {
		if pages[i] != pages[i-1]+1 {
			ok = false
			break
		}
	}
	if ok {
		return fmt.Sprintf("%d-%d", pages[0], pages[len(pages)-1])
	}
	return fmt.Sprintf("%d..%d", pages[0], pages[len(pages)-1])
}
