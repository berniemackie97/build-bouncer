package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCreatePreservesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("version: 1\nchecks:\n  - name: old\n    run: echo old\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Checks: []Check{{Name: "new", Run: "echo new"}}}
	if err := Create(path, cfg); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected existing-file error, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(original) {
		t.Fatalf("existing config modified: data=%q err=%v", data, err)
	}
}

func TestSaveReplacesExistingConfigWithoutPartialWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &Config{Checks: []Check{{Name: "first", Run: "echo ok"}}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Checks[0].Run = "echo changed"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	actual, err := Load(path)
	if err != nil || actual.Checks[0].Run != "echo changed" {
		t.Fatalf("config read after replacement failed: %+v %v", actual, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected temporary or backup files: %v %v", entries, err)
	}
}

func TestInvalidConfigSaveCannotClobberExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("original")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Checks: []Check{{Name: "invalid", Run: ""}}}
	if err := Save(path, cfg); err == nil {
		t.Fatal("invalid config must not be saved")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != "original" {
		t.Fatalf("config was damaged: %q %v", after, err)
	}
}

func TestSaveNewConfigIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &Config{Checks: []Check{{Name: "test", Run: "echo ok"}}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("new config mode = %v", info.Mode().Perm())
	}
}

func TestSaveDoesNotFollowConfigSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra permissions on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "outside")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.yaml")
	if err := os.Symlink(target, configPath); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Checks: []Check{{Name: "test", Run: "echo ok"}}}
	if err := Save(configPath, cfg); err == nil {
		t.Fatal("symlink must be rejected")
	}
	content, err := os.ReadFile(target)
	if err != nil || !strings.EqualFold(string(content), "private") {
		t.Fatalf("symlink target was modified: %q %v", content, err)
	}
}
