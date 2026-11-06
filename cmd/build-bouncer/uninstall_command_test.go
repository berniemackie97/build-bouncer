package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/cli"
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
	if code := runUninstallWithOptions(uninstallOptions{purge: true, yes: true}, ctx); code != exitOK {
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

func TestUninstallDefaultPreservesData(t *testing.T) {
	repo := withTempRepo(t)
	configPath := filepath.Join(repo, ".buildbouncer", "config.yaml")
	statePath := filepath.Join(repo, ".git", "build-bouncer", "state.json")
	for path, contents := range map[string]string{configPath: "custom config", statePath: "private state"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
		t.Fatalf("hook install: %d %s", code, stderr)
	}
	var stdout, stderr bytes.Buffer
	if code := runUninstall(false, cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitOK {
		t.Fatalf("uninstall failed: exit=%d stderr=%s", code, stderr.String())
	}
	for _, path := range []string{configPath, statePath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("default uninstall deleted data %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatalf("hook not removed: %v", err)
	}
}

func TestUninstallDryRunAndPurgeConfirmation(t *testing.T) {
	repo := withTempRepo(t)
	path := filepath.Join(repo, ".buildbouncer", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
		t.Fatalf("hook install: %d %s", code, stderr)
	}
	hookPath := filepath.Join(repo, ".git", "hooks", "pre-push")
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runUninstallWithOptions(uninstallOptions{purge: true}, ctx); code != exitUsage {
		t.Fatalf("unconfirmed purge succeeded: %d", code)
	}
	if code := runUninstallWithOptions(uninstallOptions{purge: true, dryRun: true}, ctx); code != exitOK {
		t.Fatalf("dry-run failed: %d %s", code, stderr.String())
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("dry-run removed hook: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "user config" {
		t.Fatalf("dry-run damaged configuration: %q %v", data, err)
	}
	if !strings.Contains(stdout.String(), "Would purge:") || !strings.Contains(stdout.String(), "Would remove hook:") {
		t.Fatalf("incomplete preview: %q", stdout.String())
	}
}

func TestUninstallReportsFailureForUnexpectedLegacyDirectory(t *testing.T) {
	repo := withTempRepo(t)
	path := filepath.Join(repo, ".buildbouncer.yaml")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "user-data"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runUninstallWithOptions(uninstallOptions{purge: true, yes: true}, cli.Context{Stdout: &stdout, Stderr: &stderr})
	if code != exitUsage {
		t.Fatalf("purge should report cleanup error: exit=%d stdout=%q", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "non-regular configuration") {
		t.Fatalf("missing cleanup error: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(path, "user-data")); err != nil {
		t.Fatalf("foreign data was deleted: %v", err)
	}
}

func TestUninstallDoesNotRemoveForeignHookWithoutForce(t *testing.T) {
	repo := withTempRepo(t)
	hook := filepath.Join(repo, ".git", "hooks", "pre-push")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho foreign\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runUninstallWithOptions(uninstallOptions{purge: true, yes: true}, cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage {
		t.Fatalf("expected refusal, got %d", code)
	}
	if _, err := os.Stat(hook); err != nil {
		t.Fatalf("foreign hook removed: %v", err)
	}
}
