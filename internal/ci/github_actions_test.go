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

func TestNormalizeWorkingDirectory(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "github.workspace with forward slash",
			input: "${{github.workspace}}/build",
			want:  "build",
		},
		{
			name:  "github.workspace with backslash",
			input: "${{github.workspace}}\\build",
			want:  "build",
		},
		{
			name:  "github.workspace with spaces and forward slash",
			input: "${{ github.workspace }}/build",
			want:  "build",
		},
		{
			name:  "github.workspace with spaces and backslash",
			input: "${{ github.workspace }}\\build",
			want:  "build",
		},
		{
			name:  "github.workspace only",
			input: "${{github.workspace}}",
			want:  "",
		},
		{
			name:  "github.workspace with spaces only",
			input: "${{ github.workspace }}",
			want:  "",
		},
		{
			name:  "normal path without template",
			input: "build",
			want:  "build",
		},
		{
			name:  "nested path",
			input: "${{github.workspace}}/foo/bar/baz",
			want:  "foo/bar/baz",
		},
		{
			name:  "just slash",
			input: "/",
			want:  "",
		},
		{
			name:  "just backslash",
			input: "\\",
			want:  "",
		},
		{
			name:  "whitespace",
			input: "   ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWorkingDirectory(tt.input)
			if got != tt.want {
				t.Errorf("normalizeWorkingDirectory(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestChecksFromGitHubActionsWithWorkspaceTemplate(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
jobs:
  build:
    steps:
      - name: Run tests
        run: ctest -C Release --output-on-failure --verbose
        working-directory: ${{github.workspace}}/build
`
	path := filepath.Join(workflowDir, "workspace.yml")
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
	if checks[0].Cwd != "build" {
		t.Fatalf("expected cwd to be 'build', got %q", checks[0].Cwd)
	}
}

func TestNormalizeGitHubActionsTemplates(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		wantFunc func(got string) bool
		wantDesc string
	}{
		{
			name:    "no templates",
			command: "go test ./...",
			wantFunc: func(got string) bool {
				return got == "go test ./..."
			},
			wantDesc: "unchanged",
		},
		{
			name:    "matrix.build_type with spaces",
			command: "cmake --build build --config ${{ matrix.build_type }}",
			wantFunc: func(got string) bool {
				return got == "cmake --build build --config Release"
			},
			wantDesc: "matrix.build_type replaced with Release",
		},
		{
			name:    "matrix.build_type without spaces",
			command: "ctest -C ${{matrix.build_type}}",
			wantFunc: func(got string) bool {
				return got == "ctest -C Release"
			},
			wantDesc: "matrix.build_type replaced with Release",
		},
		{
			name:    "VCPKG_ROOT env var",
			command: "cmake -DCMAKE_TOOLCHAIN_FILE=$env:VCPKG_ROOT/scripts/buildsystems/vcpkg.cmake",
			wantFunc: func(got string) bool {
				// Should either use env var value or common path, or remain unchanged if not found
				return got != "" && !strings.Contains(got, "$env:VCPKG_ROOT") || got == "cmake -DCMAKE_TOOLCHAIN_FILE=/scripts/buildsystems/vcpkg.cmake"
			},
			wantDesc: "VCPKG_ROOT resolved or removed",
		},
		{
			name:    "GITHUB_WORKSPACE env var",
			command: "echo $env:GITHUB_WORKSPACE",
			wantFunc: func(got string) bool {
				// Should be replaced with current directory
				return !strings.Contains(got, "$env:GITHUB_WORKSPACE")
			},
			wantDesc: "GITHUB_WORKSPACE replaced with cwd",
		},
		{
			name:    "regular env var",
			command: "echo $env:PATH",
			wantFunc: func(got string) bool {
				return got == "echo $env:PATH"
			},
			wantDesc: "regular env vars unchanged",
		},
		{
			name:    "multiline with templates",
			command: "cmake --preset default `\n  -DCMAKE_BUILD_TYPE=${{ matrix.build_type }}",
			wantFunc: func(got string) bool {
				return strings.Contains(got, "Release") && !strings.Contains(got, "${{")
			},
			wantDesc: "templates replaced in multiline",
		},
		{
			name:    "github context variables",
			command: "echo ${{ github.sha }}",
			wantFunc: func(got string) bool {
				return got == "echo "
			},
			wantDesc: "github.* variables removed",
		},
		{
			name:    "env context variables",
			command: "echo ${{ env.MY_VAR }}",
			wantFunc: func(got string) bool {
				return got == "echo "
			},
			wantDesc: "env.* variables removed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeGitHubActionsTemplates(tt.command)
			if !tt.wantFunc(got) {
				t.Errorf("normalizeGitHubActionsTemplates(%q) = %q, want %s", tt.command, got, tt.wantDesc)
			}
		})
	}
}

func TestChecksFromGitHubActionsTransformsTemplateExpressions(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}

	content := `
jobs:
  build:
    steps:
      - name: Simple check
        run: echo "hello"
      - name: With matrix variable
        run: cmake --build build --config ${{ matrix.build_type }}
      - name: With VCPKG
        run: echo VCPKG=$env:VCPKG_ROOT
      - name: Another simple check
        run: go test ./...
`
	path := filepath.Join(workflowDir, "transform.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatalf("ChecksFromGitHubActions error: %v", err)
	}

	// Should get all 4 checks, with templates transformed
	if len(checks) != 4 {
		t.Fatalf("expected 4 checks (all transformed), got %d", len(checks))
	}

	// Verify matrix.build_type was transformed
	found := false
	for _, check := range checks {
		if strings.Contains(check.Run, "cmake --build") {
			found = true
			if !strings.Contains(check.Run, "Release") {
				t.Errorf("check %q should have matrix.build_type replaced with Release, got: %q", check.Name, check.Run)
			}
			if strings.Contains(check.Run, "${{") {
				t.Errorf("check %q still contains template syntax: %q", check.Name, check.Run)
			}
		}
	}
	if !found {
		t.Error("expected to find cmake --build check")
	}
}
