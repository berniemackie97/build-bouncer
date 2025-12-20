package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailLinesTrimsAndLimits(t *testing.T) {
	input := "a\nb\nc\n\n"
	got := TailLines(input, 2)
	if got != "b\nc" {
		t.Fatalf("unexpected tail: %q", got)
	}
}

func TestSanitizeStripsBadChars(t *testing.T) {
	got := sanitize(" hi!!/there ")
	if got != "hi___there" {
		t.Fatalf("unexpected sanitized value: %q", got)
	}
}

func TestResolveDefaultLogDirUsesGitWhenPresent(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}
	dir := resolveDefaultLogDir(root)
	if !strings.Contains(dir, filepath.Join(".git", "build-bouncer", "logs")) {
		t.Fatalf("expected git log dir, got %q", dir)
	}
}
