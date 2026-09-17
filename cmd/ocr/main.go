package main

import (
	"errors"
	"flag"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	"github.com/kelvins-io/eino-repository-rag/internal/ocr"
)

func main() {
	defer logger.Sync()

	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	_ = os.Setenv("OMP_THREAD_LIMIT", "1")
	_ = os.Setenv("OMP_NUM_THREADS", "1")

	srv := &http.Server{
		Addr:              *addr,
		Handler:           ocr.NewServer().Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}
	tess, tessErr := exec.LookPath("tesseract")
	ppm, ppmErr := exec.LookPath("pdftoppm")
	if tessErr != nil {
		logger.S().Fatal("[ocr] missing tesseract in PATH")
	}
	if ppmErr != nil {
		logger.S().Fatal("[ocr] missing pdftoppm in PATH")
	}
	logger.S().Infof("[ocr] listening on %s max_procs=%s omp_thread_limit=1 tesseract=%s pdftoppm=%s",
		*addr, getenvDefault("OCR_MAX_PROCS", "1"), tess, ppm)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.S().Fatal(err)
	}
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
