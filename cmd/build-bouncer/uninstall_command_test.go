package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"build-bouncer/internal/cli"
)

func TestUninstallRemovesArtifacts(t *testing.T) {
	repo := withTempRepo(t)

	cfgPath := filepath.Join(repo, ".buildbouncer", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	content := `
version: 1
checks:
  - name: "lint"
    run: "echo ok"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".buildbouncer", "assets", "insults"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".buildbouncer", "assets", "insults", "default.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write insults: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(repo, ".git", "build-bouncer", "logs"), 0o755); err != nil {
		t.Fatalf("mkdir git logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "build-bouncer", "logs", "x.log"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	if code, _, stderr := runHookCmd([]string{"install"}); code != exitOK {
		t.Fatalf("install hook exit=%d stderr=%q", code, stderr)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runUninstall(false, ctx); code != exitOK {
		t.Fatalf("uninstall exit=%d stderr=%q", code, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(repo, ".buildbouncer")); !os.IsNotExist(err) {
		t.Fatalf("expected .buildbouncer to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "build-bouncer")); !os.IsNotExist(err) {
		t.Fatalf("expected .git/build-bouncer to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatalf("expected hook removed, stat err=%v", err)
	}
}
