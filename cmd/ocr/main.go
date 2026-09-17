package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/ocr"
)

func main() {
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
		log.Fatal("[ocr] missing tesseract in PATH")
	}
	if ppmErr != nil {
		log.Fatal("[ocr] missing pdftoppm in PATH")
	}
	log.Printf("[ocr] listening on %s max_procs=%s omp_thread_limit=1 tesseract=%s pdftoppm=%s",
		*addr, getenvDefault("OCR_MAX_PROCS", "1"), tess, ppm)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
