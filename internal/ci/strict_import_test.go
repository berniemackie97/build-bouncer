package ci

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func importWorkflow(t *testing.T, contents string) ([]string, error) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "check.yml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	checks, err := ChecksFromGitHubActions(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(checks))
	for i, check := range checks {
		out[i] = check.Run
	}
	return out, nil
}

func TestImportRejectsUnsupportedWorkflowSemantics(t *testing.T) {
	cases := []struct {
		name, workflow, failure string
	}{
		{"matrix expression", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ matrix.build_type }}\n", "run"},
		{"github expression", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ github.sha }}\n", "run"},
		{"environment expression", "env:\n  CI: ${{ github.ref }}\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n", "workflow env"},
		{"step environment", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n        env:\n          TOKEN: ${{ secrets.API_TOKEN }}\n", "env"},
		{"unknown condition", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    if: github.ref == 'refs/heads/main'\n    steps:\n      - run: echo ok\n", "condition"},
		{"compound condition", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - if: runner.os == 'Windows' && success()\n        run: echo ok\n", "condition"},
		{"matrix condition", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    if: matrix.os == 'ubuntu-latest'\n    steps:\n      - run: echo ok\n", "condition"},
		{"unknown matrix dimension", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        build_type: [Release, Debug]\n    steps:\n      - run: echo ok\n", "matrix dimension"},
		{"duplicate OS matrix", "jobs:\n  build:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-22.04, ubuntu-24.04]\n    steps:\n      - run: echo ok\n", "multiple runner images"},
		{"unrecognized action", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: vendor/action@v1\n      - run: echo ok\n", "unsupported action"},
		{"docker action", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker://alpine:latest\n      - run: echo ok\n", "unsupported action"},
		{"unknown shell", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n        shell: 'bash --noprofile {0}'\n", "shell"},
		{"unknown runner", "jobs:\n  build:\n    runs-on: private-runner\n    steps:\n      - run: echo ok\n", "runs-on"},
		{"missing runner", "jobs:\n  build:\n    steps:\n      - run: echo ok\n", "runs-on is required"},
		{"unbound OS matrix", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        os: [ubuntu-latest, windows-latest]\n    steps:\n      - run: echo ok\n", "matrix.os must determine runs-on"},
		{"multi-label runner", "jobs:\n  build:\n    runs-on: [self-hosted, linux]\n    steps:\n      - run: echo ok\n", "multi-label"},
		{"job dependency", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    needs: prepare\n    steps:\n      - run: echo ok\n", "dependencies"},
		{"job container", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    container: ubuntu:latest\n    steps:\n      - run: echo ok\n", "containers"},
		{"job service", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    services:\n      db:\n        image: postgres:17\n    steps:\n      - run: echo ok\n", "services"},
		{"job timeout", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps:\n      - run: echo ok\n", "job timeouts"},
		{"step timeout", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n        timeout-minutes: 10\n", "timeouts"},
		{"continue on error", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n        continue-on-error: true\n", "continue-on-error"},
		{"unknown working directory", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n        working-directory: ${{ inputs.path }}\n", "working-directory"},
		{"invalid runs-on type", "jobs:\n  build:\n    runs-on:\n      group: hosted\n    steps:\n      - run: echo ok\n", "runs-on"},
		{"nonscalar env", "jobs:\n  build:\n    runs-on: ubuntu-latest\n    env:\n      OPTIONS: [one, two]\n    steps:\n      - run: echo ok\n", "scalar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := importWorkflow(t, tc.workflow)
			if err == nil || !strings.Contains(err.Error(), tc.failure) {
				t.Fatalf("expected error containing %q, got %v", tc.failure, err)
			}
		})
	}
}

func TestImportPreservesLiteralScriptsAndInheritedDefaults(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `name: Check
env:
  SHARED: workflow
  OVERRIDE: workflow
defaults:
  run:
    working-directory: ${{ github.workspace }}/src
    shell: bash
jobs:
  zebra:
    runs-on: ubuntu-latest
    env:
      OVERRIDE: job
    defaults:
      run:
        working-directory: app
    steps:
      - name: step two
        run: echo $env:VCPKG_ROOT
        env:
          OVERRIDE: step
      - name: step three
        run: echo hello
        working-directory: ${{github.workspace}}/subdir
  alpha:
    runs-on: ubuntu-latest
    steps:
      - name: step one
        run: go test ./...
`
	if err := os.WriteFile(filepath.Join(dir, "checks.yml"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || !strings.Contains(first[0].Name, ":alpha:") {
		t.Fatalf("jobs not ordered deterministically: %+v", first)
	}
	if first[0].Cwd != "src" || first[0].Shell != "bash" || first[0].Env["SHARED"] != "workflow" {
		t.Fatalf("workflow defaults lost: %+v", first[0])
	}
	if first[1].Cwd != "app" || first[1].Env["OVERRIDE"] != "step" || first[1].Run != "echo $env:VCPKG_ROOT" {
		t.Fatalf("step overrides or literal script changed: %+v", first[1])
	}
	if first[2].Cwd != "subdir" || first[2].Env["OVERRIDE"] != "job" {
		t.Fatalf("job defaults lost: %+v", first[2])
	}
	for i := 0; i < 30; i++ {
		next, err := ChecksFromGitHubActions(root)
		if err != nil || !reflect.DeepEqual(next, first) {
			t.Fatalf("import %d changed: %+v %v", i, next, err)
		}
	}
}

func TestImportRejectsDuplicateCheckNames(t *testing.T) {
	source := `name: Build
jobs:
  first:
    runs-on: ubuntu-latest
    name: compile
    steps:
      - name: tests
        run: go test ./...
  second:
    runs-on: ubuntu-latest
    name: compile
    steps:
      - name: tests
        run: go vet ./...
`
	_, err := importWorkflow(t, source)
	if err == nil || !strings.Contains(err.Error(), "duplicate imported check name") {
		t.Fatalf("expected duplicate name rejection, got %v", err)
	}
}

func TestImportDoesNotRewriteCommands(t *testing.T) {
	script := `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "$env:VCPKG_ROOT"
      - run: echo "$GITHUB_WORKSPACE"
`
	checks, err := importWorkflow(t, script)
	if err != nil || len(checks) != 2 {
		t.Fatalf("failed to import literal scripts: %v %+v", err, checks)
	}
	if checks[0] != `echo "$env:VCPKG_ROOT"` || checks[1] != `echo "$GITHUB_WORKSPACE"` {
		t.Fatalf("import changed original commands: %+v", checks)
	}
}
