package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
    runs-on: ubuntu-latest
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
			{Name: "ci:old:step", Run: "echo old", Source: "ci:old"},
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
    runs-on: ubuntu-latest
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

func TestCISyncIsIdempotentAndPreservesUnownedChecks(t *testing.T) {
	repo := withTempRepo(t)
	path := config.DefaultConfigPath(repo)
	cfg := &config.Config{
		Checks: []config.Check{
			{Name: "ci:manual", Run: "echo manual"},
			{Name: "ci:stale", Run: "echo stale", Source: "ci:previous"},
			{Name: "local", Run: "go vet ./..."},
		},
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflow := "jobs:\n  z:\n    runs-on: ubuntu-latest\n    steps:\n      - name: lint\n        run: go vet ./...\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - name: test\n        run: go test ./...\n"
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	context := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if exit := runCISync(context); exit != exitOK {
		t.Fatalf("initial sync failed: %d %s", exit, stderr.String())
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var custom, generated, stale bool
	for _, check := range updated.Checks {
		switch check.Name {
		case "ci:manual":
			custom = true
		case "ci:stale":
			stale = true
		default:
			if strings.HasPrefix(check.Source, "ci:") {
				generated = true
			}
		}
	}
	if !custom || !generated || stale {
		t.Fatalf("unexpected ownership after sync: %+v", updated.Checks)
	}
	for i := 0; i < 3; i++ {
		stdout.Reset()
		stderr.Reset()
		if exit := runCISync(context); exit != exitOK {
			t.Fatalf("repeat sync %d failed: %d %s", i, exit, stderr.String())
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(first, after) {
			t.Fatalf("sync %d modified stable configuration: %v", i, err)
		}
	}
}

func TestCISyncRejectedExpressionDoesNotModifyConfig(t *testing.T) {
	repo := withTempRepo(t)
	path := config.DefaultConfigPath(repo)
	cfg := &config.Config{Checks: []config.Check{{Name: "ci:existing", Run: "go test ./...", Source: "ci:old"}}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflow := "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ github.sha }}\n"
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runCISync(cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage {
		t.Fatalf("unsafe workflow imported: %d", code)
	}
	if !strings.Contains(stderr.String(), "unsupported GitHub Actions expression") {
		t.Fatalf("missing actionable import error: %s", stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected workflow damaged existing configuration: %v", err)
	}
}

func TestCISyncPreservesAllMatrixVariants(t *testing.T) {
	repo := withTempRepo(t)
	path := config.DefaultConfigPath(repo)
	if err := config.Save(path, &config.Config{Checks: []config.Check{{Name: "local", Run: "echo local"}}}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflow := `name: Build
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
    steps:
      - name: Test
        run: go test ./...
`
	if err := os.WriteFile(filepath.Join(dir, "test.yml"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("CI sync failed: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Checks) != 4 {
		t.Fatalf("expected custom check and three platform variants, got %+v", cfg.Checks)
	}
	wantShell := map[string]string{"linux": "bash", "windows": "pwsh", "macos": "bash"}
	seen := make(map[string]bool)
	for _, check := range cfg.Checks[1:] {
		if len(check.OS) != 1 || check.Shell != wantShell[check.OS[0]] || seen[check.OS[0]] || check.ID == "" {
			t.Fatalf("invalid platform-specific check: %+v", check)
		}
		seen[check.OS[0]] = true
	}
	if len(seen) != 3 {
		t.Fatalf("missing platforms after synchronization: %v", seen)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("repeat sync failed: %d %s", code, stderr.String())
	}
	second, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("CI sync was not idempotent: %v", err)
	}
}

func TestCISyncDoesNotSuppressMandatoryCheckWithOptionalLocalPolicy(t *testing.T) {
	repo := withTempRepo(t)
	cfgPath := config.DefaultConfigPath(repo)
	manual := config.Check{
		Name: "local-optional", Run: "go test ./...", Shell: "bash",
		OS: config.StringList{"linux"}, Requires: config.StringList{"go"},
		AllowFailure: true,
	}
	if err := config.Save(cfgPath, &config.Config{
		Checks: []config.Check{manual}, Protection: config.Protection{Level: "lax"},
	}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: required\n        run: go test ./...\n"
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("CI sync failed: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Checks) != 2 || !cfg.Checks[0].AllowFailure || cfg.Checks[1].AllowFailure || !isCICheck(cfg.Checks[1]) {
		t.Fatalf("required imported check was suppressed: %+v", cfg.Checks)
	}
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("repeat CI sync failed: %d %s", code, stderr.String())
	}
	after, err := os.ReadFile(cfgPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("repeated CI sync changed config: %v", err)
	}
}

func TestCISyncRefusesModifiedOwnedPolicyWithoutWrites(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*config.Check)
	}{
		{name: "allow failure", change: func(c *config.Check) { c.AllowFailure = true }},
		{name: "timeout", change: func(c *config.Check) { c.Timeout = time.Minute }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := withTempRepo(t)
			path := config.DefaultConfigPath(repo)
			dir := filepath.Join(repo, ".github", "workflows")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			workflow := "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: required\n        run: go test ./...\n"
			if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(workflow), 0o644); err != nil {
				t.Fatal(err)
			}
			checks := stampGeneratedChecks([]config.Check{{Name: "ci:ci:test:linux:required", Source: "ci:ci", Run: "go test ./...", Shell: "bash", OS: config.StringList{"linux"}}}, "ci")
			tc.change(&checks[0])
			if err := config.Save(path, &config.Config{Checks: checks}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if code := runCISync(cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage || !strings.Contains(stderr.String(), "has been modified") {
				t.Fatalf("CI sync accepted edited policy: exit=%d stderr=%q", code, stderr.String())
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("rejected CI sync changed config: %v", err)
			}
		})
	}
}
