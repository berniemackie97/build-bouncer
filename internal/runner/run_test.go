package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"build-bouncer/internal/config"
)

func TestRunOneSuccessRemovesLog(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	outcome, err := runOne(context.Background(), root, root, 0, "echo", "echo hello", "", nil, 0, Options{})
	if err != nil {
		t.Fatalf("runOne error: %v", err)
	}
	if outcome.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", outcome.ExitCode)
	}
	if outcome.LogPath != "" {
		t.Fatalf("expected no log path on success, got %q", outcome.LogPath)
	}
	if !strings.Contains(strings.ToLower(outcome.Tail), "hello") {
		t.Fatalf("expected tail to contain output, got %q", outcome.Tail)
	}
}

func TestRunOneFailureKeepsLog(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	cmd := "echo nope && exit 3"

	outcome, err := runOne(context.Background(), root, root, 1, "fail", cmd, "", nil, 0, Options{})
	if err != nil {
		t.Fatalf("runOne error: %v", err)
	}
	if outcome.ExitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if outcome.LogPath == "" {
		t.Fatal("expected log path on failure")
	}
	if _, statErr := os.Stat(outcome.LogPath); statErr != nil {
		t.Fatalf("expected log file to exist, got error: %v", statErr)
	}
	if !strings.Contains(strings.ToLower(outcome.Tail), "nope") {
		t.Fatalf("expected tail output on failure, got %q", outcome.Tail)
	}
}

func TestRunOneTimeout(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	cmd := sleepCommand(2)
	outcome, err := runOne(context.Background(), root, root, 2, "timeout", cmd, "", nil, 200*time.Millisecond, Options{})
	if err != nil {
		t.Fatalf("runOne error: %v", err)
	}
	if !outcome.TimedOut {
		t.Fatalf("expected timeout, got %+v", outcome)
	}
	if outcome.Timeout <= 0 {
		t.Fatalf("expected timeout duration, got %v", outcome.Timeout)
	}
	if outcome.LogPath == "" {
		t.Fatal("expected log path on timeout")
	}
}

func TestRunAllReportFailFastCancelsRemaining(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "first", Run: failCommand("first")},
			{Name: "second", Run: sleepCommand(2)},
			{Name: "third", Run: sleepCommand(2)},
		},
	}

	opts := Options{
		MaxParallel: 1,
		FailFast:    true,
	}

	rep, err := RunAllReport(root, cfg, opts)
	if err != nil {
		t.Fatalf("RunAllReport error: %v", err)
	}
	if len(rep.Failures) != 1 || rep.Failures[0] != "first" {
		t.Fatalf("expected first failure, got %+v", rep.Failures)
	}
	if len(rep.Canceled) != 2 || rep.Canceled[0] != "second" || rep.Canceled[1] != "third" {
		t.Fatalf("expected canceled checks [second third], got %+v", rep.Canceled)
	}
}

func sleepCommand(seconds int) string {
	if seconds < 1 {
		seconds = 1
	}
	if runtime.GOOS == "windows" {
		return "powershell -NoProfile -Command \"Start-Sleep -Seconds " + strconv.Itoa(seconds) + "\""
	}
	return "sleep " + strconv.Itoa(seconds)
}

func failCommand(msg string) string {
	if runtime.GOOS == "windows" {
		return "echo " + msg + " && exit /b 1"
	}
	return "echo " + msg + " && exit 1"
}
