package service

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kelvins-io/eino-repository-rag/internal/model"
)

func TestNormalizeImportSources(t *testing.T) {
	long := strings.Repeat("a", 1100)
	tests := []struct {
		name string
		raw  []string
		n    int
		want []string
	}{
		{"empty", nil, 3, []string{"", "", ""}},
		{"broadcast", []string{" https://a "}, 3, []string{"https://a", "https://a", "https://a"}},
		{"aligned", []string{"u1", "u2", "u3"}, 3, []string{"u1", "u2", "u3"}},
		{"partial", []string{"u1", "u2"}, 3, []string{"u1", "u2", ""}},
		{"truncate", []string{long}, 1, []string{long[:1024]}},
		{"zero files", []string{"u1"}, 0, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeImportSources(tt.raw, tt.n)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeImportTitles(t *testing.T) {
	long := strings.Repeat("t", 300)
	tests := []struct {
		name string
		raw  []string
		n    int
		want []string
	}{
		{"empty", nil, 2, []string{"", ""}},
		{"no broadcast", []string{" 仅首个 "}, 3, []string{"仅首个", "", ""}},
		{"aligned", []string{"a", "b"}, 2, []string{"a", "b"}},
		{"truncate", []string{long}, 1, []string{long[:256]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeImportTitles(tt.raw, tt.n)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestSourceUploadExists(t *testing.T) {
	if sourceUploadExists("") || sourceUploadExists("   ") {
		t.Fatal("empty path should be missing")
	}
	dir := t.TempDir()
	if sourceUploadExists(dir) {
		t.Fatal("directory should not count as upload file")
	}
	file := filepath.Join(dir, "a.md")
	if sourceUploadExists(file) {
		t.Fatal("missing file")
	}
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !sourceUploadExists(file) {
		t.Fatal("expected existing file")
	}

	docs := []*model.Document{
		{FilePath: file},
		{FilePath: filepath.Join(dir, "missing.md")},
		{FilePath: ""},
		nil,
	}
	attachSourceAvailable(docs)
	if !docs[0].SourceAvailable {
		t.Fatal("existing file should be available")
	}
	if docs[1].SourceAvailable || docs[2].SourceAvailable {
		t.Fatal("missing or empty path should not be available")
	}
}
