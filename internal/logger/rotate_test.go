package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyWriterRotatesAcrossMidnight(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	cur := time.Date(2026, 9, 19, 23, 50, 0, 0, time.Local)
	w, err := openDailyWriter(path, 30, func() time.Time { return cur })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("yesterday\n")); err != nil {
		t.Fatal(err)
	}
	cur = time.Date(2026, 9, 20, 0, 1, 0, 0, time.Local)
	if _, err := w.Write([]byte("today\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	yest, err := os.ReadFile(filepath.Join(dir, "app-2026-09-19.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(yest) != "yesterday\n" {
		t.Fatalf("yesterday archive=%q", yest)
	}
	today, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(today) != "today\n" {
		t.Fatalf("today log=%q", today)
	}
}

func TestDailyWriterArchivesStaleFileOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	yesterday := time.Date(2026, 9, 19, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(path, yesterday, yesterday); err != nil {
		t.Fatal(err)
	}
	cur := time.Date(2026, 9, 20, 8, 0, 0, 0, time.Local)
	w, err := openDailyWriter(path, 30, func() time.Time { return cur })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	old, err := os.ReadFile(filepath.Join(dir, "app-2026-09-19.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(old) != "old\n" {
		t.Fatalf("archive=%q", old)
	}
	curBody, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(curBody) != "new\n" {
		t.Fatalf("current=%q", curBody)
	}
}

func TestDailyWriterDropsExpiredArchives(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	keep := filepath.Join(dir, "app-2026-09-14.log")
	drop := filepath.Join(dir, "app-2026-09-13.log")
	collision := filepath.Join(dir, "app-2026-09-01.1.log")
	unrelated := filepath.Join(dir, "app-notes.log")
	for _, name := range []string{keep, drop, collision, unrelated} {
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cur := time.Date(2026, 9, 20, 9, 0, 0, 0, time.Local)
	w, err := openDailyWriter(path, 7, func() time.Time { return cur })
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("expected to keep %s: %v", keep, err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("expected to keep unrelated file: %v", err)
	}
	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Fatalf("expected expired archive removed, err=%v", err)
	}
	if _, err := os.Stat(collision); !os.IsNotExist(err) {
		t.Fatalf("expected numbered archive removed, err=%v", err)
	}
}
