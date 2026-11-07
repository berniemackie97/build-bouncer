package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestInitPreservesExistingConfigAndPacks(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	old := []byte("preserve my custom configuration")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	pack := filepath.Join(config.DefaultAssetsPath(root), "insults", "default.json")
	if err := os.MkdirAll(filepath.Dir(pack), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pack, []byte("custom pack"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runInit(false, false, "go", cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage {
		t.Fatalf("expected explicit-force error, got %d, stderr=%q", code, stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(old) {
		t.Fatalf("custom config changed: %q %v", data, err)
	}
	data, err = os.ReadFile(pack)
	if err != nil || string(data) != "custom pack" {
		t.Fatalf("custom pack changed: %q %v", data, err)
	}
}

func TestInitRequiresForceToReplaceConfiguration(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runInit(true, false, "go", cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitOK {
		t.Fatalf("force init failed: exit=%d stderr=%q", code, stderr.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Meta.Template.ID != "go" || len(cfg.Checks) == 0 {
		t.Fatalf("force init did not replace template config: %+v", cfg)
	}
	if !strings.Contains(stdout.String(), "Created:") {
		t.Fatalf("missing success message: %q", stdout.String())
	}
}

func TestInitPreservesLegacyConfig(t *testing.T) {
	root := withTempRepo(t)
	path := config.LegacyConfigPath(root)
	if err := os.WriteFile(path, []byte("legacy preferences"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runInit(false, false, "go", cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage {
		t.Fatalf("expected refusal for existing legacy config, got %d stderr=%q", code, stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "legacy preferences" {
		t.Fatalf("legacy config overwritten: %q %v", data, err)
	}
	if _, err := os.Stat(config.DefaultConfigPath(root)); !os.IsNotExist(err) {
		t.Fatalf("unexpected new config: %v", err)
	}
}
