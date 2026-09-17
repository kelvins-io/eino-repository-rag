package ocr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHealthAndImagePDFHTTP(t *testing.T) {
	srv := NewServer()
	srv.Image = func(ctx context.Context, path, langs string, psm int) (string, error) {
		if path == "" || langs == "" || psm <= 0 {
			t.Fatalf("bad image args path=%q langs=%q psm=%d", path, langs, psm)
		}
		return "图片正文", nil
	}
	srv.PDF = func(ctx context.Context, pdfPath string, pages []int, dpi int, langs string, psm int) (map[int]string, error) {
		if len(pages) != 2 || pages[0] != 1 || pages[1] != 3 {
			t.Fatalf("pages=%v", pages)
		}
		return map[int]string{1: "第一页", 3: "第三页"}, nil
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("health status=%d", resp.StatusCode)
	}

	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	if err := os.WriteFile(img, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pdf := filepath.Join(dir, "a.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewClient(ts.URL, 0, "chi_sim+eng", 6)
	text, err := c.Recognize(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	if text != "图片正文" {
		t.Fatalf("image text=%q", text)
	}

	pages, err := c.RecognizePDF(context.Background(), pdf, []int{1, 3}, 200)
	if err != nil {
		t.Fatal(err)
	}
	if pages[1] != "第一页" || pages[3] != "第三页" {
		t.Fatalf("pdf pages=%v", pages)
	}
}

func TestFormatBytesAndPageRange(t *testing.T) {
	if got := formatBytes(512); got != "512B" {
		t.Fatalf("bytes=%s", got)
	}
	if got := formatBytes(2048); got != "2.0KB" {
		t.Fatalf("kb=%s", got)
	}
	if got := formatBytes(2 << 20); got != "2.0MB" {
		t.Fatalf("mb=%s", got)
	}
	if got := pageRange(nil); got != "-" {
		t.Fatalf("empty=%s", got)
	}
	if got := pageRange([]int{3}); got != "3" {
		t.Fatalf("single=%s", got)
	}
	if got := pageRange([]int{1, 2, 3}); got != "1-3" {
		t.Fatalf("range=%s", got)
	}
	if got := pageRange([]int{1, 3}); got != "1..3" {
		t.Fatalf("sparse=%s", got)
	}
}

func TestJobContext(t *testing.T) {
	if jobOf(nil) != "ocr" {
		t.Fatal(jobOf(nil))
	}
	id := nextJobID("pdf")
	ctx := withJob(context.Background(), id)
	if got := jobOf(ctx); got != id {
		t.Fatalf("job=%s want=%s", got, id)
	}
	if got := jobOf(context.Background()); got != "ocr" {
		t.Fatalf("default=%s", got)
	}
}

func TestTesseractEnvLimitsOpenMP(t *testing.T) {
	env := tesseractEnv()
	found := false
	for _, e := range env {
		if e == "OMP_THREAD_LIMIT=1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected OMP_THREAD_LIMIT=1")
	}
}

func TestClientUnreachable(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", 0, "", 0)
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := c.Recognize(context.Background(), p)
	if err == nil {
		t.Fatal("expected unreachable error")
	}
}
