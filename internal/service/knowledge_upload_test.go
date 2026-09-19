package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kelvins-io/eino-repository-rag/internal/model"
)

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
