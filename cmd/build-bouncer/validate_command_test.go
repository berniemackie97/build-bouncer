package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"build-bouncer/internal/cli"
)

func TestValidateCommand(t *testing.T) {
	repo := withTempRepo(t)

	cfgPath := filepath.Join(repo, ".buildbouncer", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	content := `
version: 1
checks:
  - name: "lint"
    run: "go vet ./..."
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runValidate("", ctx); code != exitOK {
		t.Fatalf("validate exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Config OK:") {
		t.Fatalf("expected config ok output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Checks: 1") {
		t.Fatalf("expected checks count output, got %q", stdout.String())
	}
}
