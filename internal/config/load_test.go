package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultsAndValidation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1

checks:
  - name: "lint"
    run: "go vet ./..."
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Insults.File != ".buildbouncer/assets/insults/default.json" {
		t.Fatalf("expected default insults file, got %q", cfg.Insults.File)
	}
	if cfg.Banter.File != ".buildbouncer/assets/banter/default.json" {
		t.Fatalf("expected default banter file, got %q", cfg.Banter.File)
	}
	if cfg.Insults.Locale != "en" || cfg.Banter.Locale != "en" {
		t.Fatalf("expected default locales to be en, got insults=%q banter=%q", cfg.Insults.Locale, cfg.Banter.Locale)
	}
	if cfg.Version != 1 {
		t.Fatalf("expected version 1, got %d", cfg.Version)
	}
}

func TestLoadFailsWithNoChecks(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1
checks: []
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error for missing checks, got nil")
	}
}

func TestLoadFailsOnUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 2
checks:
  - name: t
    run: echo hi
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
}

func TestLoadFailsWithShellArguments(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1
checks:
  - name: bad-shell
    run: echo hi
    shell: "bash -lc"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err == nil || !strings.Contains(err.Error(), "shell") {
		t.Fatalf("expected shell validation error, got %v", err)
	}
}

func TestLoadAllowsShellPathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1
checks:
  - name: ok-shell
    run: echo hi
    shell: 'C:\Program Files\Git\bin\bash.exe'
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err != nil {
		t.Fatalf("expected shell path to be accepted, got %v", err)
	}
}
