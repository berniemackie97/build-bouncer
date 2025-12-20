package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunOneSuccessRemovesLog(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	exitCode, tail, logPath, err := runOne(root, root, 0, "echo", "echo hello", nil, Options{})
	if err != nil {
		t.Fatalf("runOne error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if logPath != "" {
		t.Fatalf("expected no log path on success, got %q", logPath)
	}
	if !strings.Contains(strings.ToLower(tail), "hello") {
		t.Fatalf("expected tail to contain output, got %q", tail)
	}
}

func TestRunOneFailureKeepsLog(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	cmd := "echo nope && exit 3"

	exitCode, tail, logPath, err := runOne(root, root, 1, "fail", cmd, nil, Options{})
	if err != nil {
		t.Fatalf("runOne error: %v", err)
	}
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if logPath == "" {
		t.Fatal("expected log path on failure")
	}
	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Fatalf("expected log file to exist, got error: %v", statErr)
	}
	if !strings.Contains(strings.ToLower(tail), "nope") {
		t.Fatalf("expected tail output on failure, got %q", tail)
	}
}
