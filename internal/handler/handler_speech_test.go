package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
	"github.com/kelvins-io/eino-repository-rag/internal/tts"
)

type stubSpeaker struct {
	text   string
	result tts.Result
	err    error
}

func (s *stubSpeaker) Synthesize(_ context.Context, text string) (tts.Result, error) {
	s.text = text
	return s.result, s.err
}

func TestSynthesizeSpeechOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubSpeaker{result: tts.Result{Audio: []byte("ID3ok"), ContentType: "audio/mpeg"}}
	h := NewKnowledgeHandler(nil, config.RAGConfig{MaxUploadFileSizeMB: 50, MaxUploadFiles: 5})
	h.WithTTS(stub)

	body, _ := json.Marshal(map[string]string{"text": "证券合规要求是什么"})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chat/speech", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.SynthesizeSpeech(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("ct=%s", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != "ID3ok" {
		t.Fatalf("audio=%q", rec.Body.String())
	}
	if stub.text != "证券合规要求是什么" {
		t.Fatalf("text=%s", stub.text)
	}
}

func TestSynthesizeSpeechDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewKnowledgeHandler(nil, config.RAGConfig{MaxUploadFileSizeMB: 50, MaxUploadFiles: 5})
	body, _ := json.Marshal(map[string]string{"text": "你好"})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chat/speech", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.SynthesizeSpeech(c)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSynthesizeSpeechEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubSpeaker{err: tts.ErrEmptyText}
	h := NewKnowledgeHandler(nil, config.RAGConfig{MaxUploadFileSizeMB: 50, MaxUploadFiles: 5})
	h.WithTTS(stub)
	body, _ := json.Marshal(map[string]string{"text": "   "})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chat/speech", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.SynthesizeSpeech(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}
