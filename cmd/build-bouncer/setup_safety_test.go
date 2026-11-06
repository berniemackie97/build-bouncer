package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestSetupRestoresNewConfigAfterForeignHookRejection(t *testing.T) {
	repo := withTempRepo(t)
	hook := filepath.Join(repo, ".git", "hooks", "pre-push")
	original := []byte("#!/bin/sh\necho foreign\n")
	if err := os.WriteFile(hook, original, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runSetup(false, false, false, "go", ctx); code != exitUsage {
		t.Fatalf("setup should fail for foreign hook, exit=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Lstat(config.ConfigDir(repo)); !os.IsNotExist(err) {
		t.Fatalf("new config and assets should have been rolled back: %v", err)
	}
	data, err := os.ReadFile(hook)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("foreign hook modified: %q %v", data, err)
	}
	if strings.Contains(stdout.String(), "Created:") {
		t.Fatalf("failed setup announced a committed config: %q", stdout.String())
	}
}

func TestSetupRestoresExistingFilesWhenBinaryCannotBeInstalled(t *testing.T) {
	repo := withTempRepo(t)
	cfgPath := config.DefaultConfigPath(repo)
	assets := config.DefaultAssetsPath(repo)
	insult := filepath.Join(assets, "insults", "default.json")
	banter := filepath.Join(assets, "banter", "default.json")
	original := map[string][]byte{
		cfgPath: []byte("custom config"),
		insult:  []byte("custom insults"),
		banter:  []byte("custom banter"),
	}
	for path, content := range original {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	hook := filepath.Join(repo, ".git", "hooks", "pre-push")
	foreign := []byte("#!/bin/sh\necho foreign\n")
	if err := os.WriteFile(hook, foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(repo, ".git", "hooks", "bin", "build-bouncer")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if err := os.MkdirAll(binary, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runSetup(true, false, false, "go", cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage {
		t.Fatalf("setup should fail when binary destination is a directory, exit=%d stderr=%q", code, stderr.String())
	}
	for path, expected := range original {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, expected) {
			t.Fatalf("rollback failed for %s: got=%q err=%v", path, got, err)
		}
	}
	got, err := os.ReadFile(hook)
	if err != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("prior hook not restored: got=%q err=%v", got, err)
	}
	entries, err := os.ReadDir(filepath.Dir(binary))
	if err != nil || len(entries) != 1 {
		t.Fatalf("staged executable leaked: %v err=%v", entries, err)
	}
}

func TestInitRejectsSymlinkedManagedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires extra permissions on Windows")
	}
	repo := withTempRepo(t)
	outside := t.TempDir()
	link := filepath.Join(repo, ".buildbouncer")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runInit(false, "go", cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage {
		t.Fatalf("expected refusal for symlinked config directory, exit=%d stderr=%q", code, stderr.String())
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unexpected changes outside repository: %v %v", entries, err)
	}
}

func TestInitFailsBeforeWritingWhenTemplateIsInvalid(t *testing.T) {
	repo := withTempRepo(t)
	var stdout, stderr bytes.Buffer
	if code := runInit(false, "unrecognized-template", cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage {
		t.Fatalf("expected unsupported template rejection, exit=%d", code)
	}
	if _, err := os.Lstat(config.ConfigDir(repo)); !os.IsNotExist(err) {
		t.Fatalf("invalid init created config directory: %v", err)
	}
}
