package asr

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

func TestTranscribeSuccess(t *testing.T) {
	var gotModel, gotLang, gotPrompt, gotFilename string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer test-key") {
			t.Fatalf("auth=%s", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		gotModel = r.FormValue("model")
		gotLang = r.FormValue("language")
		gotPrompt = r.FormValue("prompt")
		_, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		gotFilename = hdr.Filename
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":" 证券合规要求是什么 "}`))
	}))
	t.Cleanup(ts.Close)

	c := NewClient(config.ASRConfig{
		APIKey:         "test-key",
		BaseURL:        ts.URL + "/v1",
		Model:          "FunAudioLLM/SenseVoiceSmall",
		Language:       "zh",
		Prompt:         "知识库问答",
		TimeoutSeconds: 5,
	})
	text, err := c.Transcribe(context.Background(), Request{
		Filename:    "speech.webm",
		ContentType: "audio/webm",
		Body:        strings.NewReader("fake-audio"),
		Prompt:      "证劵合规",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text != "证券合规要求是什么" {
		t.Fatalf("text=%q", text)
	}
	if gotModel != "FunAudioLLM/SenseVoiceSmall" || gotLang != "zh" {
		t.Fatalf("model=%s lang=%s", gotModel, gotLang)
	}
	if gotPrompt != "知识库问答 证劵合规" {
		t.Fatalf("prompt=%q", gotPrompt)
	}
	if gotFilename != "speech.webm" {
		t.Fatalf("filename=%s", gotFilename)
	}
}

func TestTranscribeNotConfigured(t *testing.T) {
	c := NewClient(config.ASRConfig{})
	_, err := c.Transcribe(context.Background(), Request{Body: strings.NewReader("x")})
	if !strings.Contains(err.Error(), "未配置") {
		t.Fatalf("err=%v", err)
	}
}

func TestTranscribeEmptyAudio(t *testing.T) {
	c := NewClient(config.ASRConfig{
		APIKey:  "k",
		BaseURL: "http://example.invalid/v1",
		Model:   "m",
	})
	_, err := c.Transcribe(context.Background(), Request{Body: strings.NewReader("")})
	if err != ErrEmptyAudio {
		t.Fatalf("err=%v", err)
	}
}

func TestTranscribeUpstreamError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"invalid audio"}`)
	}))
	t.Cleanup(ts.Close)

	c := NewClient(config.ASRConfig{
		APIKey:         "k",
		BaseURL:        ts.URL,
		Model:          "m",
		TimeoutSeconds: 5,
	})
	_, err := c.Transcribe(context.Background(), Request{Body: strings.NewReader("abc")})
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.Status != 400 {
		t.Fatalf("upstream=%v", err)
	}
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitizeFilename("../a/b.webm"); got != "b.webm" {
		t.Fatalf("got=%s", got)
	}
	if got := sanitizeFilename(""); got != "speech.webm" {
		t.Fatalf("empty=%s", got)
	}
}
