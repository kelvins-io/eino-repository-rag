package rag

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveIndexedUploadDeletesFileAndEmptyParents(t *testing.T) {
	root := t.TempDir()
	kbDir := filepath.Join(root, "1", "9")
	if err := os.MkdirAll(kbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(kbDir, "doc.md")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeIndexedUpload(root, file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "1")); !os.IsNotExist(err) {
		t.Fatalf("expected empty tenant dir removed, stat err=%v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("upload_dir should remain: %v", err)
	}
}

func TestRemoveIndexedUploadKeepsSiblingFiles(t *testing.T) {
	root := t.TempDir()
	kbDir := filepath.Join(root, "1", "9")
	if err := os.MkdirAll(kbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(kbDir, "a.md")
	keep := filepath.Join(kbDir, "b.md")
	if err := os.WriteFile(target, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeIndexedUpload(root, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("target should be gone")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("sibling should remain: %v", err)
	}
}

func TestRemoveIndexedUploadRefusesOutsideUploadDir(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeIndexedUpload(root, outside); err == nil {
		t.Fatal("expected refusal")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file should remain: %v", err)
	}
}

func TestRemoveIndexedUploadMissingFileIsOK(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "1", "gone.md")
	if err := removeIndexedUpload(root, missing); err != nil {
		t.Fatal(err)
	}
}

func TestPathWithinDir(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "1", "a.md")
	if _, _, ok := pathWithinDir(inside, root); !ok {
		t.Fatal("expected inside")
	}
	if _, _, ok := pathWithinDir(filepath.Join(root, "..", "x"), root); ok {
		t.Fatal("expected outside")
	}
}
