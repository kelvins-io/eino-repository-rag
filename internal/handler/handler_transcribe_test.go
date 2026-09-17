package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kelvins-io/eino-repository-rag/internal/asr"
	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

type stubTranscriber struct {
	text string
	err  error
	req  asr.Request
}

func (s *stubTranscriber) Transcribe(_ context.Context, req asr.Request) (string, error) {
	s.req = req
	if req.Body != nil {
		_, _ = req.Body.Read(make([]byte, 8))
	}
	return s.text, s.err
}

func TestTranscribeSpeechOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubTranscriber{text: "这个知识库怎么用"}
	h := NewKnowledgeHandler(nil, config.RAGConfig{MaxUploadFileSizeMB: 50, MaxUploadFiles: 5})
	h.WithASR(stub, 8<<20)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", "speech.webm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("fake-audio")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("prompt", "电影资料"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chat/transcribe", body)
	c.Request.Header.Set("Content-Type", w.FormDataContentType())

	h.TranscribeSpeech(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp.Data.(map[string]any)
	if data["text"] != "这个知识库怎么用" {
		t.Fatalf("data=%v", resp.Data)
	}
	if stub.req.Prompt != "电影资料" {
		t.Fatalf("prompt=%s", stub.req.Prompt)
	}
}

func TestTranscribeSpeechDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewKnowledgeHandler(nil, config.RAGConfig{MaxUploadFileSizeMB: 50, MaxUploadFiles: 5})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chat/transcribe", strings.NewReader(""))
	h.TranscribeSpeech(c)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTranscribeSpeechRejectsNonAudio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewKnowledgeHandler(nil, config.RAGConfig{MaxUploadFileSizeMB: 50, MaxUploadFiles: 5})
	h.WithASR(&stubTranscriber{text: "x"}, 8<<20)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chat/transcribe", body)
	c.Request.Header.Set("Content-Type", w.FormDataContentType())
	h.TranscribeSpeech(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}
