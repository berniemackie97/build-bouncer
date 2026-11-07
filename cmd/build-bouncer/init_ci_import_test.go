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

const complexDotNetWorkflow = `name: CI
jobs:
  quality:
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    steps:
      - name: Restore
        run: dotnet restore OpenConquer.Client.slnx --locked-mode
      - name: Conditional validation
        if: steps.assets.outputs.available == 'true'
        run: dotnet test OpenConquer.Client.slnx --no-restore
  cross-platform:
    name: ${{ matrix.name }}
    runs-on: ${{ matrix.runner }}
    timeout-minutes: 15
    strategy:
      fail-fast: false
      matrix:
        include:
          - name: Windows
            runner: windows-2025
          - name: macOS
            runner: macos-15
    steps:
      - name: Build
        run: dotnet build OpenConquer.Client.slnx --configuration Release
`

func writeCIWorkflow(t *testing.T, root, source string) string {
	t.Helper()
	path := filepath.Join(root, ".github", "workflows", "ci.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInitDotNetWorksWithUnsupportedGitHubWorkflow(t *testing.T) {
	root := withTempRepo(t)
	writeDotNetFixture(t, root, false)
	workflow := writeCIWorkflow(t, root, complexDotNetWorkflow)
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}

	if code := newInitCommand().Run(ctx, []string{"--dotnet"}); code != exitOK {
		t.Fatalf("local initialization failed: exit=%d stderr=%q", code, stderr.String())
	}

	cfg, err := config.Load(config.DefaultConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Meta.Template.ID != "dotnet" || len(cfg.Checks) != 4 {
		t.Fatalf("unexpected local .NET configuration: %+v", cfg)
	}
	for _, check := range cfg.Checks {
		if check.Source != "template:dotnet" {
			t.Fatalf("CI check was imported without explicit request: %+v", check)
		}
	}
	if code := runValidate("", ctx); code != exitOK {
		t.Fatalf("generated configuration failed validation: %d %s", code, stderr.String())
	}
	after, err := os.ReadFile(workflow)
	if err != nil || string(after) != complexDotNetWorkflow {
		t.Fatalf("existing GitHub Actions workflow modified: %v", err)
	}

	original, err := os.ReadFile(config.DefaultConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := runCISync(ctx); code != exitUsage || !strings.Contains(stderr.String(), "not supported by CI import") {
		t.Fatalf("explicit CI sync did not reject unsupported workflow: exit=%d stderr=%q", code, stderr.String())
	}
	current, err := os.ReadFile(config.DefaultConfigPath(root))
	if err != nil || !bytes.Equal(original, current) {
		t.Fatalf("rejected CI sync changed local configuration: %v", err)
	}
}

func TestInitExplicitImportRejectsComplexWorkflowWithoutWrites(t *testing.T) {
	root := withTempRepo(t)
	workflow := writeCIWorkflow(t, root, complexDotNetWorkflow)
	var stdout, stderr bytes.Buffer
	if code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr}, []string{"--dotnet", "--import-ci"}); code != exitUsage ||
		!strings.Contains(stderr.String(), "not supported by CI import") {
		t.Fatalf("unsupported import did not fail: exit=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Lstat(config.ConfigDir(root)); !os.IsNotExist(err) {
		t.Fatalf("failed explicit import created managed files: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatalf("failed explicit import installed a hook: %v", err)
	}
	if data, err := os.ReadFile(workflow); err != nil || string(data) != complexDotNetWorkflow {
		t.Fatalf("failed import changed existing workflow: %v", err)
	}
}

func TestInitExplicitImportIncludesSupportedChecks(t *testing.T) {
	root := withTempRepo(t)
	writeDotNetFixture(t, root, false)
	writeCIWorkflow(t, root, "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: check\n        run: echo ok\n")
	var stdout, stderr bytes.Buffer
	if code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr}, []string{"--dotnet", "--import-ci"}); code != exitOK {
		t.Fatalf("supported CI import failed: exit=%d stderr=%q", code, stderr.String())
	}
	cfg, err := config.Load(config.DefaultConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	var ciChecks int
	for _, check := range cfg.Checks {
		if strings.HasPrefix(check.Source, "ci:") {
			ciChecks++
		}
	}
	if ciChecks != 1 {
		t.Fatalf("expected one explicitly imported CI check, got %+v", cfg.Checks)
	}
}

func TestSetupWithUnsupportedWorkflowDoesNotImportCI(t *testing.T) {
	root := withTempRepo(t)
	workflow := writeCIWorkflow(t, root, complexDotNetWorkflow)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "project.go"), []byte("package project\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := newSetupCommand().Run(ctx, []string{"--go", "--no-copy"}); code != exitOK {
		t.Fatalf("local setup failed: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := config.Load(config.DefaultConfigPath(root)); err != nil {
		t.Fatalf("setup did not produce valid configuration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "pre-push")); err != nil {
		t.Fatalf("setup did not install pre-push hook: %v", err)
	}
	if after, err := os.ReadFile(workflow); err != nil || string(after) != complexDotNetWorkflow {
		t.Fatalf("setup changed original GitHub Actions workflow: %v", err)
	}
}

func TestSetupExplicitCIImportFailureDoesNotInstallHook(t *testing.T) {
	root := withTempRepo(t)
	writeCIWorkflow(t, root, complexDotNetWorkflow)
	var stdout, stderr bytes.Buffer
	if code := newSetupCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr}, []string{"--go", "--import-ci"}); code != exitUsage ||
		!strings.Contains(stderr.String(), "not supported by CI import") {
		t.Fatalf("unsupported setup import did not fail: exit=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(config.DefaultConfigPath(root)); !os.IsNotExist(err) {
		t.Fatalf("failed setup import left configuration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatalf("failed setup import installed pre-push hook: %v", err)
	}
}
