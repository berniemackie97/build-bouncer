package runner

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestBoundedLogWriterDrainsAfterLimit(t *testing.T) {
	var log bytes.Buffer
	writer := &boundedLogWriter{writer: &log, limit: 4}
	data := []byte("abcdefghij")
	n, err := writer.Write(data)
	if err != nil || n != len(data) || log.String() != "abcd" || !writer.truncated {
		t.Fatalf("unexpected log state: n=%d err=%v written=%q truncated=%v", n, err, log.String(), writer.truncated)
	}
	n, err = writer.Write([]byte("more output"))
	if err != nil || n != len("more output") || log.String() != "abcd" {
		t.Fatalf("output after cap was not fully consumed: n=%d err=%v log=%q", n, err, log.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, os.ErrPermission
}

func TestBoundedLogWriterRecordsStorageError(t *testing.T) {
	writer := &boundedLogWriter{writer: failingWriter{}, limit: 128}
	for range 2 {
		if n, err := writer.Write([]byte("something")); err != nil || n != len("something") {
			t.Fatalf("check output must be drained after storage failure: n=%d err=%v", n, err)
		}
	}
	if !errors.Is(writer.err, os.ErrPermission) {
		t.Fatalf("storage error not retained: %v", writer.err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestBoundedLogWriterDetectsShortWrite(t *testing.T) {
	writer := &boundedLogWriter{writer: shortWriter{}, limit: 128}
	_, _ = writer.Write([]byte("abc"))
	if !errors.Is(writer.err, io.ErrShortWrite) {
		t.Fatalf("short writes must surface as errors: %v", writer.err)
	}
}

func TestRunAllReportTruncatedLog(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Checks: []config.Check{{Name: "broken", Run: failCommand("abcdefghijklmnopqrstuv")}}}
	report, err := RunAllReport(root, cfg, Options{MaxLogBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.Failures[0] != "broken" {
		t.Fatalf("expected failed check, got %+v", report)
	}
	if !report.LogTruncated["broken"] {
		t.Fatalf("expected truncated log marker: %+v", report)
	}
	path := report.LogFiles["broken"]
	if path == "" {
		t.Fatal("expected failure log")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 8 || !strings.Contains(report.FailureTails["broken"], "abcdefghijklmnopqrstuv") {
		t.Fatalf("log cap or unbounded tail incorrect: log=%q tail=%q", contents, report.FailureTails["broken"])
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("saved log must be private, got %v", info.Mode().Perm())
		}
		directory, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		if directory.Mode().Perm() != 0o700 {
			t.Fatalf("default log directory must be private, got %v", directory.Mode().Perm())
		}
	}
}

func TestPruneCheckLogsLeavesUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	write := func(name string, age time.Duration) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
		date := now.Add(-age)
		if err := os.Chtimes(path, date, date); err != nil {
			t.Fatal(err)
		}
	}
	write("build-bouncer-stale.log", 30*24*time.Hour)
	write("build-bouncer-recent1.log", 72*time.Hour)
	write("build-bouncer-recent2.log", 48*time.Hour)
	write("build-bouncer-recent3.log", 24*time.Hour)
	write("personal.log", 30*24*time.Hour)
	write("unrelated.txt", 30*24*time.Hour)

	if err := pruneCheckLogs(dir, logLimits{maxBytes: 128, keep: 2, days: 7}, now); err != nil {
		t.Fatal(err)
	}
	for name, keep := range map[string]bool{
		"build-bouncer-stale.log":   false,
		"build-bouncer-recent1.log": false,
		"build-bouncer-recent2.log": true,
		"build-bouncer-recent3.log": true,
		"personal.log":              true,
		"unrelated.txt":             true,
	} {
		_, err := os.Stat(filepath.Join(dir, name))
		if keep && err != nil || !keep && !os.IsNotExist(err) {
			t.Errorf("%s expected keep=%v, stat=%v", name, keep, err)
		}
	}
}

func TestRunAllReportHonorsRetentionCount(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Checks: []config.Check{
		{Name: "first", Run: failCommand("first")},
		{Name: "second", Run: failCommand("second")},
		{Name: "third", Run: failCommand("third")},
	}}
	report, err := RunAllReport(root, cfg, Options{MaxSavedLogs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 3 || len(report.LogFiles) != 2 {
		t.Fatalf("must retain two logs without losing failure results: %+v", report)
	}
	if len(report.FailureTails) != 3 {
		t.Fatalf("missing in-memory failure tails: %+v", report.FailureTails)
	}
	entries, err := os.ReadDir(resolveDefaultLogDir(root))
	if err != nil || len(entries) != 2 {
		t.Fatalf("expected two retained logs, got %d err=%v", len(entries), err)
	}
}

func TestInvalidLogBudgetsFailBeforeExecution(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Checks: []config.Check{{Name: "pass", Run: "echo ok"}}}
	for _, opts := range []Options{
		{MaxLogBytes: 65 << 20},
		{MaxSavedLogs: 101},
		{LogRetentionDays: 366},
		{MaxLogBytes: -1},
	} {
		if _, err := RunAllReport(root, cfg, opts); err == nil {
			t.Fatalf("invalid limits must fail: %+v", opts)
		}
	}
}

func TestBoundedOutputDoesNotFailPassingCheck(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Checks: []config.Check{{Name: "pass", Run: "echo 0123456789abcdefghijklmnop"}}}
	report, err := RunAllReport(root, cfg, Options{MaxLogBytes: 4})
	if err != nil || len(report.Failures) > 0 {
		t.Fatalf("truncation must not change successful check outcome: report=%+v err=%v", report, err)
	}
	entries, err := os.ReadDir(resolveDefaultLogDir(root))
	if err != nil || len(entries) != 0 {
		t.Fatalf("successful checks should not retain logs: %v %v", entries, err)
	}
}

func TestPrepareLogDirDoesNotChangeCustomDirectoryMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := prepareLogDir(path, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o750 {
		t.Fatalf("custom directory mode changed to %v", info.Mode().Perm())
	}
}

func TestLargeOutputUsesBoundedMemoryAndDisk(t *testing.T) {
	tail := newLimitedBuffer(128 * 1024)
	var retained bytes.Buffer
	writer := &boundedLogWriter{writer: &retained, limit: 1 << 20}
	stream := io.MultiWriter(tail, writer)
	chunk := bytes.Repeat([]byte("x"), 64<<10)
	for range 1024 {
		n, err := stream.Write(chunk)
		if err != nil || n != len(chunk) {
			t.Fatalf("large output not fully consumed: n=%d err=%v", n, err)
		}
	}
	if retained.Len() != 1<<20 || len(tail.buffer) != 128*1024 || !writer.truncated || writer.err != nil {
		t.Fatalf("unexpected retained sizes: disk=%d tail=%d truncated=%v err=%v",
			retained.Len(), len(tail.buffer), writer.truncated, writer.err)
	}
}
