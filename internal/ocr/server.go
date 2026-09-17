package ocr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxUpload = 512 << 20

type imageResponse struct {
	Text string `json:"text"`
}

type pdfPageResponse struct {
	Page  int    `json:"page"`
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

type pdfResponse struct {
	Pages []pdfPageResponse `json:"pages"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Server HTTP OCR 服务（容器内跑 tesseract / pdftoppm）。
type Server struct {
	Image func(ctx context.Context, path, langs string, psm int) (string, error)
	PDF   func(ctx context.Context, pdfPath string, pages []int, dpi int, langs string, psm int) (map[int]string, error)
}

// NewServer 使用本机 tesseract / pdftoppm。
func NewServer() *Server {
	return &Server{
		Image: func(ctx context.Context, path, langs string, psm int) (string, error) {
			return RecognizeImage(ctx, Config{
				Languages:   langs,
				PageSegMode: psm,
				Timeout:     120 * time.Second,
			}, path)
		},
		PDF: func(ctx context.Context, pdfPath string, pages []int, dpi int, langs string, psm int) (map[int]string, error) {
			return RecognizePDFPages(ctx, Config{
				Languages:   langs,
				PageSegMode: psm,
				DPI:         dpi,
				Concurrency: 1,
				Timeout:     120 * time.Second,
			}, pdfPath, pages)
		},
	}
}

// Handler 路由：GET /health、POST /v1/ocr/image、POST /v1/ocr/pdf。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/ocr/image", s.handleImage)
	mux.HandleFunc("/v1/ocr/pdf", s.handlePDF)
	return mux
}

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	up, cleanup, err := saveUpload(r, "file")
	if err != nil {
		log.Printf("[ocr] image reject: %v", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer cleanup()

	langs := formOr(r, "languages", "chi_sim+eng")
	psm, _ := strconv.Atoi(formOr(r, "psm", "6"))
	job := nextJobID("image")
	ctx := withJob(r.Context(), job)
	log.Printf("[%s] start file=%s size=%s langs=%s psm=%d", job, up.Name, formatBytes(up.Size), langs, psm)
	t0 := time.Now()
	text, err := s.Image(ctx, up.Path, langs, psm)
	dur := time.Since(t0).Round(time.Millisecond)
	if err != nil {
		log.Printf("[%s] fail dur=%s err=%v", job, dur, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("[%s] done runes=%d dur=%s", job, len([]rune(text)), dur)
	writeJSON(w, http.StatusOK, imageResponse{Text: text})
}

func (s *Server) handlePDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	up, cleanup, err := saveUpload(r, "file")
	if err != nil {
		log.Printf("[ocr] pdf reject: %v", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer cleanup()

	pages, err := parsePages(formOr(r, "pages", ""))
	if err != nil {
		log.Printf("[ocr] pdf reject: %v", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(pages) == 0 {
		log.Printf("[ocr] pdf reject: pages is required")
		writeError(w, http.StatusBadRequest, "pages is required")
		return
	}
	langs := formOr(r, "languages", "chi_sim+eng")
	psm, _ := strconv.Atoi(formOr(r, "psm", "6"))
	dpi, _ := strconv.Atoi(formOr(r, "dpi", "200"))
	job := nextJobID("pdf")
	ctx := withJob(r.Context(), job)
	log.Printf("[%s] start file=%s size=%s pages=%d range=%s dpi=%d langs=%s",
		job, up.Name, formatBytes(up.Size), len(pages), pageRange(pages), dpi, langs)
	t0 := time.Now()
	out, err := s.PDF(ctx, up.Path, pages, dpi, langs, psm)
	ok, empty := 0, 0
	for _, p := range pages {
		if strings.TrimSpace(out[p]) != "" {
			ok++
		} else {
			empty++
		}
	}
	dur := time.Since(t0).Round(time.Millisecond)
	if err != nil && ok == 0 {
		log.Printf("[%s] fail pages=%d dur=%s err=%v", job, len(pages), dur, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err != nil {
		log.Printf("[%s] done ok=%d empty=%d dur=%s partial_err=%v", job, ok, empty, dur, err)
	} else {
		log.Printf("[%s] done ok=%d empty=%d dur=%s", job, ok, empty, dur)
	}
	resp := pdfResponse{Pages: make([]pdfPageResponse, 0, len(pages))}
	for _, p := range pages {
		item := pdfPageResponse{Page: p, Text: strings.TrimSpace(out[p])}
		if item.Text == "" && err != nil {
			item.Error = err.Error()
		}
		resp.Pages = append(resp.Pages, item)
	}
	writeJSON(w, http.StatusOK, resp)
}

type savedFile struct {
	Path string
	Name string
	Size int64
}

func saveUpload(r *http.Request, field string) (savedFile, func(), error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		return savedFile{}, func() {}, fmt.Errorf("parse multipart: %w", err)
	}
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return savedFile{}, func() {}, fmt.Errorf("missing %s", field)
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if ext == "" {
		ext = ".bin"
	}
	tmp, err := os.CreateTemp("", "ocr-up-*"+ext)
	if err != nil {
		return savedFile{}, func() {}, err
	}
	n, err := io.Copy(tmp, f)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return savedFile{}, func() {}, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return savedFile{}, func() {}, err
	}
	name := filepath.Base(hdr.Filename)
	if name == "" || name == "." {
		name = filepath.Base(tmp.Name())
	}
	path := tmp.Name()
	return savedFile{Path: path, Name: name, Size: n}, func() { _ = os.Remove(path) }, nil
}

func parsePages(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var pages []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid page %q", part)
		}
		pages = append(pages, n)
	}
	return pages, nil
}

func formOr(r *http.Request, key, fallback string) string {
	v := strings.TrimSpace(r.FormValue(key))
	if v == "" {
		return fallback
	}
	return v
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errorResponse{Error: msg})
}
