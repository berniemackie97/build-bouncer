package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestRealMixedWorkspaceSmoke(t *testing.T) {
	if os.Getenv("BUILDBOUNCER_REAL_WORKSPACE_SMOKE") != "1" {
		t.Skip("requires Go, .NET 10, Node, pnpm, and Git")
	}

	source, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "build-bouncer")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	smokeExec(t, source, "go", "build", "-o", binary, ".")

	root := t.TempDir()
	smokeExec(t, root, "git", "init", "-q")
	apiDir := filepath.Join(root, "services", "api")
	workerDir := filepath.Join(root, "services", "worker")
	for _, dir := range []string{apiDir, workerDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	smokeExec(t, apiDir, "dotnet", "new", "sln", "-n", "Api", "--format", "slnx")
	smokeExec(t, apiDir, "dotnet", "new", "mstest", "-n", "Api.Tests", "-o", "Api.Tests", "--no-restore")
	smokeExec(t, apiDir, "dotnet", "sln", "Api.slnx", "add", "Api.Tests/Api.Tests.csproj")
	smokeExec(t, workerDir, "dotnet", "new", "sln", "-n", "Worker", "--format", "slnx")

	const manifest = `{"name":"smoke-root","private":true,"packageManager":"pnpm@10.0.0","scripts":{"lint":"node scripts/verify.cjs lint","test":"node scripts/verify.cjs test"}}`
	smokeWrite(t, root, "package.json", manifest)
	smokeWrite(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	smokeWrite(t, root, "turbo.json", "{}\n")
	smokeWrite(t, root, "scripts/verify.cjs", `const fs = require('node:fs');
if (!fs.existsSync('services/api/Api.slnx')) process.exit(42);
fs.appendFileSync('node-runs.txt', process.argv[2] + '\n');
`)
	smokeWrite(t, root, "packages/dashboard/package.json", `{"name":"dashboard","scripts":{"lint":"node -e \"process.exit(41)\""}}`)
	smokeWrite(t, root, "apps/mobile/package.json", `{"name":"mobile","scripts":{"test":"node -e \"process.exit(43)\""}}`)

	out := smokeExec(t, root, binary, "setup", "--dotnet", "--node", "--solution", "services/api/Api.slnx", "--ci")
	if !strings.Contains(out, "All checks passed") {
		t.Fatalf("setup did not run checks: %s", out)
	}
	cfg, err := config.Load(config.DefaultConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Meta.Template.ID != "dotnet+node" || cfg.Meta.Inputs["dotnet.solution"] != "services/api/Api.slnx" ||
		len(cfg.Checks) != 6 || !cfg.Runner.FailFast || cfg.Runner.MaxParallel != 1 {
		t.Fatalf("unexpected mixed workspace config: %+v", cfg)
	}
	for _, check := range cfg.Checks[:4] {
		if !strings.Contains(check.Run, "services/api/Api.slnx") ||
			strings.Contains(check.Run, "services/worker") {
			t.Fatalf("incorrect .NET target: %+v", check)
		}
	}
	for i, name := range []string{"node-lint", "node-tests"} {
		if cfg.Checks[i+4].Name != name {
			t.Fatalf("root package selection differs: %+v", cfg.Checks)
		}
	}
	smokeExec(t, root, binary, "validate")
	smokeExec(t, root, binary, "doctor")
	assertSmokeRuns(t, root, 1)

	remote := filepath.Join(t.TempDir(), "remote.git")
	smokeExec(t, root, "git", "init", "--bare", "-q", remote)
	smokeExec(t, root, "git", "remote", "add", "origin", remote)
	smokeExec(t, root, "git", "add", "package.json", "pnpm-workspace.yaml", "turbo.json", "scripts/verify.cjs")
	smokeExec(t, root, "git", "-c", "user.name=Smoke", "-c", "user.email=smoke@example.invalid", "commit", "-qm", "Initialize workspace")
	smokeExec(t, root, "git", "push", "origin", "HEAD:refs/heads/acceptance")
	assertSmokeRuns(t, root, 2)

	hookPath := filepath.Join(root, ".git", "hooks", "pre-push")
	original, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	solution := filepath.Join(apiDir, "Api.slnx")
	if err := os.Rename(solution, solution+".disabled"); err != nil {
		t.Fatal(err)
	}
	smokeRefuse(t, root, binary, "validate")
	smokeRefuse(t, root, binary, "hook", "install", "--force", "--no-copy")
	assertSmokeHook(t, hookPath, original)
	if err := os.Rename(solution+".disabled", solution); err != nil {
		t.Fatal(err)
	}
	smokeWrite(t, root, "package.json", `{"name":"smoke-root","private":true,"packageManager":"pnpm@10.0.0","scripts":{"lint":"node scripts/verify.cjs lint"}}`)
	smokeRefuse(t, root, binary, "validate")
	smokeRefuse(t, root, binary, "hook", "install", "--force", "--no-copy")
	assertSmokeHook(t, hookPath, original)
}

func smokeExec(t *testing.T, dir, binary string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", binary, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func smokeRefuse(t *testing.T, dir, binary string, args ...string) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected %s %s to fail:\n%s", binary, strings.Join(args, " "), output)
	}
}

func smokeWrite(t *testing.T, root, relative, value string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertSmokeRuns(t *testing.T, root string, count int) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "node-runs.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), strings.Repeat("lint\ntest\n", count); got != want {
		t.Fatalf("unexpected root script execution: %q (want %q)", got, want)
	}
}

func assertSmokeHook(t *testing.T, path string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("refused admission modified installed hook: %v", err)
	}
}
