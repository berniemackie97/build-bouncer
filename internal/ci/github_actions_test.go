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
