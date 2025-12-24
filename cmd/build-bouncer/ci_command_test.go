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

func TestCISyncAddsChecks(t *testing.T) {
	repo := withTempRepo(t)

	cfgPath := filepath.Join(repo, ".buildbouncer", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: manualPlaceholderName, Run: "echo " + manualPlaceholderSnippet + " 1>&2 && exit 1"},
		},
		Insults: config.Insults{
			Mode:   "snarky",
			File:   ".buildbouncer/assets/insults/default.json",
			Locale: "en",
		},
		Banter: config.Banter{
			File:   ".buildbouncer/assets/banter/default.json",
			Locale: "en",
		},
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	workflowDir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}
	workflow := `
jobs:
  build:
    steps:
      - run: echo ci
`
	if err := os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte(workflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("ci sync exit=%d stderr=%q", code, stderr.String())
	}

	updated, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(updated.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(updated.Checks))
	}
	if updated.Checks[0].Name == manualPlaceholderName {
		t.Fatalf("expected manual placeholder removed, got %+v", updated.Checks)
	}
}

func TestCISyncReplacesOldCIChecks(t *testing.T) {
	repo := withTempRepo(t)

	cfgPath := filepath.Join(repo, ".buildbouncer", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "local", Run: "echo local"},
			{Name: "ci:old:step", Run: "echo old"},
		},
		Insults: config.Insults{
			Mode:   "snarky",
			File:   ".buildbouncer/assets/insults/default.json",
			Locale: "en",
		},
		Banter: config.Banter{
			File:   ".buildbouncer/assets/banter/default.json",
			Locale: "en",
		},
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	workflowDir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}
	workflow := `
jobs:
  build:
    steps:
      - run: echo ci
`
	if err := os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte(workflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("ci sync exit=%d stderr=%q", code, stderr.String())
	}

	updated, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if len(updated.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(updated.Checks))
	}
	if updated.Checks[0].Name != "local" && updated.Checks[1].Name != "local" {
		t.Fatalf("expected local check to remain, got %+v", updated.Checks)
	}
	for _, c := range updated.Checks {
		if strings.HasPrefix(c.Name, "ci:old") {
			t.Fatalf("expected old CI check removed, got %+v", updated.Checks)
		}
	}
}
