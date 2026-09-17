package tts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

func TestSynthesizeSuccess(t *testing.T) {
	var gotModel, gotVoice, gotInput string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer test-key") {
			t.Fatalf("auth=%s", r.Header.Get("Authorization"))
		}
		var body struct {
			Model string `json:"model"`
			Input string `json:"input"`
			Voice string `json:"voice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel, gotVoice, gotInput = body.Model, body.Voice, body.Input
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3fake-mp3"))
	}))
	t.Cleanup(ts.Close)

	c := NewClient(config.TTSConfig{
		APIKey:         "test-key",
		BaseURL:        ts.URL + "/v1",
		Model:          "FunAudioLLM/CosyVoice2-0.5B",
		Voice:          "anna",
		TimeoutSeconds: 5,
		MaxChars:       4000,
	})
	res, err := c.Synthesize(context.Background(), "引用来源[1] 证券合规要求是什么")
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Audio) != "ID3fake-mp3" {
		t.Fatalf("audio=%q", res.Audio)
	}
	if res.ContentType != "audio/mpeg" {
		t.Fatalf("ct=%s", res.ContentType)
	}
	if gotModel != "FunAudioLLM/CosyVoice2-0.5B" {
		t.Fatalf("model=%s", gotModel)
	}
	if gotVoice != "FunAudioLLM/CosyVoice2-0.5B:anna" {
		t.Fatalf("voice=%s", gotVoice)
	}
	if gotInput != "引用来源 证券合规要求是什么" {
		t.Fatalf("input=%q", gotInput)
	}
}

func TestSynthesizeNotConfigured(t *testing.T) {
	c := NewClient(config.TTSConfig{})
	_, err := c.Synthesize(context.Background(), "你好")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err=%v", err)
	}
}

func TestSynthesizeEmptyText(t *testing.T) {
	c := NewClient(config.TTSConfig{
		APIKey:  "k",
		BaseURL: "http://example.invalid/v1",
		Model:   "m",
	})
	_, err := c.Synthesize(context.Background(), "   [1]   ")
	if !errors.Is(err, ErrEmptyText) {
		t.Fatalf("err=%v", err)
	}
}

func TestSynthesizeUpstreamError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"invalid input"}`)
	}))
	t.Cleanup(ts.Close)

	c := NewClient(config.TTSConfig{
		APIKey:         "k",
		BaseURL:        ts.URL,
		Model:          "m",
		Voice:          "anna",
		TimeoutSeconds: 5,
	})
	_, err := c.Synthesize(context.Background(), "你好")
	var ue *UpstreamError
	if !errors.As(err, &ue) || ue.Status != 400 {
		t.Fatalf("upstream=%v", err)
	}
}

func TestPrepareTextTruncates(t *testing.T) {
	got := PrepareText(strings.Repeat("啊", 10), 4)
	if got != "啊啊啊啊" {
		t.Fatalf("got=%q", got)
	}
}

func TestResolveVoice(t *testing.T) {
	if got := resolveVoice("FunAudioLLM/CosyVoice2-0.5B", "anna"); got != "FunAudioLLM/CosyVoice2-0.5B:anna" {
		t.Fatalf("got=%s", got)
	}
	if got := resolveVoice("m", "m:bella"); got != "m:bella" {
		t.Fatalf("prefixed=%s", got)
	}
}
