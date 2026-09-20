package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// dailyWriter 把日志写到固定路径，并在本地日期变化时把当前文件归档为
// "<name>-YYYY-MM-DD<ext>"。进程跨天不重启也会轮转；启动时若已有文件的修改日期不是今天，也会先归档。
type dailyWriter struct {
	path   string
	maxAge int
	now    func() time.Time

	mu   sync.Mutex
	file *os.File
	day  string
}

var rotatedDatePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})(?:\.\d+)?$`)

func openDailyWriter(path string, maxAge int, now func() time.Time) (*dailyWriter, error) {
	w := &dailyWriter{path: path, maxAge: maxAge, now: now}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotateIfNeeded(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotateIfNeeded(); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

func (w *dailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *dailyWriter) currentTime() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

func (w *dailyWriter) rotateIfNeeded() error {
	now := w.currentTime()
	day := now.Format("2006-01-02")
	if w.file != nil && w.day == day {
		return nil
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
		if err := archiveLogFile(w.path, w.day); err != nil {
			return err
		}
	} else if err := archiveStaleLogFile(w.path, day, now.Location()); err != nil {
		return err
	}
	if dir := filepath.Dir(w.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create log dir %s: %w", dir, err)
		}
	}
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", w.path, err)
	}
	w.file = f
	w.day = day
	w.cleanup(now)
	return nil
}

func archiveStaleLogFile(path, today string, loc *time.Location) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("log path is a directory: %s", path)
	}
	mtimeDay := info.ModTime().In(loc).Format("2006-01-02")
	if mtimeDay == today {
		return nil
	}
	return archiveLogFile(path, mtimeDay)
}

func archiveLogFile(path, day string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dest, err := uniqueDatedPath(path, day)
	if err != nil {
		return err
	}
	if err := os.Rename(path, dest); err != nil {
		return fmt.Errorf("rotate log file %s: %w", path, err)
	}
	return nil
}

func uniqueDatedPath(path, day string) (string, error) {
	for i := 0; i < 1000; i++ {
		stamp := day
		if i > 0 {
			stamp = fmt.Sprintf("%s.%d", day, i)
		}
		dest := datedLogPath(path, stamp)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			return dest, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("too many rotated logs for %s", path)
}

func datedLogPath(path, stamp string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return base + "-" + stamp + ext
}

func (w *dailyWriter) cleanup(now time.Time) {
	if w.maxAge <= 0 {
		return
	}
	dir := filepath.Dir(w.path)
	ext := filepath.Ext(w.path)
	base := strings.TrimSuffix(filepath.Base(w.path), ext)
	prefix := base + "-"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		mid := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
		m := rotatedDatePattern.FindStringSubmatch(mid)
		if m == nil {
			continue
		}
		fileDay, err := time.ParseInLocation("2006-01-02", m[1], time.Local)
		if err != nil {
			continue
		}
		if daysBetween(fileDay, now) >= w.maxAge {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

func daysBetween(older, newer time.Time) int {
	return int(dateOnly(newer).Sub(dateOnly(older)).Hours() / 24)
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
