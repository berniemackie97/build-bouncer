package runner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultLogBytes    int64 = 4 << 20
	maxLogBytes        int64 = 64 << 20
	defaultSavedLogs         = 20
	maxSavedLogs             = 100
	defaultRetentionDays     = 7
	maxRetentionDays         = 365
)

type logLimits struct {
	maxBytes int64
	keep     int
	days     int
}

func resolveLogLimits(options Options) (logLimits, error) {
	limits := logLimits{maxBytes: options.MaxLogBytes, keep: options.MaxSavedLogs, days: options.LogRetentionDays}
	if limits.maxBytes == 0 {
		limits.maxBytes = defaultLogBytes
	}
	if limits.keep == 0 {
		limits.keep = defaultSavedLogs
	}
	if limits.days == 0 {
		limits.days = defaultRetentionDays
	}
	if limits.maxBytes < 0 || limits.maxBytes > maxLogBytes ||
		limits.keep < 0 || limits.keep > maxSavedLogs ||
		limits.days < 0 || limits.days > maxRetentionDays {
		return logLimits{}, fmt.Errorf("runner log limits must be positive and at most %d bytes, %d saved logs and %d days", maxLogBytes, maxSavedLogs, maxRetentionDays)
	}
	return limits, nil
}

func prepareLogDir(path string, owned bool) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if owned {
		return os.Chmod(path, 0o700)
	}
	return nil
}

type boundedLogWriter struct {
	writer    io.Writer
	limit     int64
	written   int64
	truncated bool
	err       error
}

func (w *boundedLogWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return len(p), nil
	}
	remaining := w.limit - w.written
	if remaining <= 0 {
		if len(p) > 0 {
			w.truncated = true
		}
		return len(p), nil
	}
	data := p
	if int64(len(p)) > remaining {
		data = p[:int(remaining)]
		w.truncated = true
	}
	n, err := w.writer.Write(data)
	w.written += int64(n)
	if err != nil {
		w.err = err
	} else if n != len(data) {
		w.err = io.ErrShortWrite
	}
	return len(p), nil
}

type logEntry struct {
	name    string
	modTime time.Time
}

func pruneCheckLogs(dir string, limits logLimits, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	cutoff := now.Add(-time.Duration(limits.days) * 24 * time.Hour)
	var existing []logEntry
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "build-bouncer-") || !strings.HasSuffix(entry.Name(), ".log") || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
			continue
		}
		existing = append(existing, logEntry{name: entry.Name(), modTime: info.ModTime()})
	}
	sort.Slice(existing, func(i, j int) bool {
		if existing[i].modTime.Equal(existing[j].modTime) {
			return existing[i].name > existing[j].name
		}
		return existing[i].modTime.After(existing[j].modTime)
	})
	for _, entry := range existing[min(limits.keep, len(existing)):] {
		if err := os.Remove(filepath.Join(dir, entry.name)); err != nil {
			return err
		}
	}
	return nil
}
