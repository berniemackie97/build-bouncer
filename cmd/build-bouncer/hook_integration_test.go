package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
)

func withTempRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("create .git dir: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir to repo: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	return repo
}

func runHookCmd(args []string) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	code := runHook(args, ctx)
	return code, stdout.String(), stderr.String()
}

func TestHookSubcommandsInstallStatusUninstall(t *testing.T) {
	repo := withTempRepo(t)

	code, _, stderr := runHookCmd([]string{"install"})
	if code != exitOK {
		t.Fatalf("install exit=%d stderr=%q", code, stderr)
	}

	hookPath := filepath.Join(repo, ".git", "hooks", "pre-push")
	hookBytes, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if !strings.Contains(string(hookBytes), "# build-bouncer pre-push hook v1") {
		t.Fatalf("expected hook marker, got: %q", string(hookBytes))
	}

	code, stdout, stderr := runHookCmd([]string{"status"})
	if code != exitOK {
		t.Fatalf("status exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "pre-push hook: installed") {
		t.Fatalf("expected status installed, got %q", stdout)
	}
	if !strings.Contains(stdout, "installed by build-bouncer: true") {
		t.Fatalf("expected status ours true, got %q", stdout)
	}
	if !strings.Contains(stdout, "copied binary present: true") {
		t.Fatalf("expected copied binary present, got %q", stdout)
	}

	code, _, stderr = runHookCmd([]string{"uninstall"})
	if code != exitOK {
		t.Fatalf("uninstall exit=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("expected hook to be removed, stat err=%v", err)
	}

	bin := filepath.Join(repo, ".git", "hooks", "bin", "build-bouncer")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Fatalf("expected copied binary to be removed, stat err=%v", err)
	}
}

func TestHookSubcommandsInstallNoCopy(t *testing.T) {
	repo := withTempRepo(t)

	code, _, stderr := runHookCmd([]string{"install", "--no-copy"})
	if code != exitOK {
		t.Fatalf("install exit=%d stderr=%q", code, stderr)
	}

	code, stdout, stderr := runHookCmd([]string{"status"})
	if code != exitOK {
		t.Fatalf("status exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "pre-push hook: installed") {
		t.Fatalf("expected status installed, got %q", stdout)
	}
	if !strings.Contains(stdout, "installed by build-bouncer: true") {
		t.Fatalf("expected status ours true, got %q", stdout)
	}
	if !strings.Contains(stdout, "copied binary present: false") {
		t.Fatalf("expected copied binary absent, got %q", stdout)
	}

	bin := filepath.Join(repo, ".git", "hooks", "bin", "build-bouncer")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Fatalf("expected no copied binary, stat err=%v", err)
	}
}

func TestHookUninstallRefusesForeignHook(t *testing.T) {
	repo := withTempRepo(t)

	hooksDir := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("create hooks dir: %v", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-push")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho not ours\n"), 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	code, _, stderr := runHookCmd([]string{"uninstall"})
	if code != exitUsage {
		t.Fatalf("expected usage exit code, got %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "not installed by build-bouncer") {
		t.Fatalf("expected foreign hook error, got %q", stderr)
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("expected hook to remain, stat err=%v", err)
	}
}
