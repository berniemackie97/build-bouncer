package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChecksFromGitHubActions(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
name: CI
jobs:
  build:
    defaults:
      run:
        working-directory: app
    env:
      FOO: bar
    steps:
      - uses: actions/checkout@v4
      - name: Run tests
        run: go test ./...
        env:
          CI: true
      - run: echo "done"
`
	path := filepath.Join(workflowDir, "ci.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatalf("ChecksFromGitHubActions error: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}

	if checks[0].Cwd != "app" || checks[1].Cwd != "app" {
		t.Fatalf("expected cwd from defaults, got %+v", checks)
	}
	if checks[0].Env["FOO"] != "bar" || checks[0].Env["CI"] != "true" {
		t.Fatalf("expected merged env, got %+v", checks[0].Env)
	}
	if !strings.HasPrefix(checks[0].Name, "ci:ci:build:") {
		t.Fatalf("unexpected check name: %q", checks[0].Name)
	}
}

func TestChecksFromGitHubActionsSkipsOtherOS(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
jobs:
  linux:
    runs-on: ubuntu-latest
    steps:
      - run: echo linux
  windows:
    runs-on: windows-latest
    steps:
      - run: echo windows
  mac:
    runs-on: macos-latest
    steps:
      - run: echo mac
`
	path := filepath.Join(workflowDir, "os.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatalf("ChecksFromGitHubActions error: %v", err)
	}
	if len(checks) != 1 {
		t.Fatalf("expected 1 check for current OS, got %d", len(checks))
	}

	wantJob := map[string]string{
		"windows": "windows",
		"linux":   "linux",
		"macos":   "mac",
	}[currentRunnerOS()]

	if wantJob != "" && !strings.Contains(checks[0].Name, ":"+wantJob+":") {
		t.Fatalf("expected check for %s job, got %q", wantJob, checks[0].Name)
	}
}

func TestChecksFromGitHubActionsMatrixRunsOn(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
jobs:
  build:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
    steps:
      - run: echo matrix
`
	path := filepath.Join(workflowDir, "matrix.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatalf("ChecksFromGitHubActions error: %v", err)
	}
	if len(checks) != 1 {
		t.Fatalf("expected 1 check for current OS, got %d", len(checks))
	}
}

func TestChecksFromGitHubActionsJobIf(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
jobs:
  gated:
    runs-on: ${{ matrix.os }}
    if: runner.os == 'Windows'
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
    steps:
      - run: echo gated
`
	path := filepath.Join(workflowDir, "if.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatalf("ChecksFromGitHubActions error: %v", err)
	}

	if currentRunnerOS() == "windows" {
		if len(checks) != 1 {
			t.Fatalf("expected 1 check on windows, got %d", len(checks))
		}
	} else if len(checks) != 0 {
		t.Fatalf("expected 0 checks on non-windows, got %d", len(checks))
	}
}

func TestChecksFromGitHubActionsStepIf(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
jobs:
  build:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
    steps:
      - run: echo always
      - if: runner.os == 'Windows'
        run: echo windows-only
`
	path := filepath.Join(workflowDir, "step-if.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatalf("ChecksFromGitHubActions error: %v", err)
	}

	if currentRunnerOS() == "windows" {
		if len(checks) != 2 {
			t.Fatalf("expected 2 checks on windows, got %d", len(checks))
		}
	} else if len(checks) != 1 {
		t.Fatalf("expected 1 check on non-windows, got %d", len(checks))
	}
}

func TestChecksFromGitHubActionsShellBash(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
jobs:
  build:
    steps:
      - name: Bash step
        shell: bash
        run: |
          echo "hello"
          echo "world"
`
	path := filepath.Join(workflowDir, "shell.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatalf("ChecksFromGitHubActions error: %v", err)
	}
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Shell != "bash" {
		t.Fatalf("expected shell bash, got %q", checks[0].Shell)
	}
	if !strings.Contains(checks[0].Run, "echo \"hello\"") {
		t.Fatalf("expected script in run, got %q", checks[0].Run)
	}
}

func TestChecksFromGitHubActionsUsesSetupNode(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
jobs:
  build:
    steps:
      - uses: actions/setup-node@v4
        with:
          cache: pnpm
      - run: pnpm test
`
	path := filepath.Join(workflowDir, "setup-node.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatalf("ChecksFromGitHubActions error: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}
	if checks[0].Run != "pnpm --version" {
		t.Fatalf("expected setup-node check, got %q", checks[0].Run)
	}
	if !strings.Contains(checks[0].Name, "setup-node") {
		t.Fatalf("expected setup-node in name, got %q", checks[0].Name)
	}
}

func TestChecksFromGitHubActionsSkipsUnusedSetupAction(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
jobs:
  build:
    steps:
      - uses: actions/setup-node@v4
      - run: go test ./...
`
	path := filepath.Join(workflowDir, "setup-unused.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatalf("ChecksFromGitHubActions error: %v", err)
	}
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if !strings.Contains(checks[0].Run, "go test") {
		t.Fatalf("expected go test check, got %q", checks[0].Run)
	}
}
