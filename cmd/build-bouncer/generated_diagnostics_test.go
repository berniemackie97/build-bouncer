package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func runGeneratedValidate(path string) (int, string) {
	var out, errOut bytes.Buffer
	code := runValidate(path, cli.Context{Stdout: &out, Stderr: &errOut})
	return code, errOut.String()
}

func TestGeneratedMixedProfileValidatesWithoutExecutingChecks(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "package.json", mixedManifest())
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	writeMixedFile(t, root, "services/api/Api.csproj", "<Project />")
	var out, errOut bytes.Buffer
	ctx := cli.Context{Stdout: &out, Stderr: &errOut}
	if code := newInitCommand().Run(ctx, []string{"--dotnet", "--node"}); code != exitOK {
		t.Fatalf("init: %d %s", code, errOut.String())
	}
	if code, report := runGeneratedValidate(""); code != exitOK {
		t.Fatalf("valid mixed profile rejected: %s", report)
	}
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
		t.Fatalf("valid mixed profile refused by hook install: %s", stderr)
	}
}

func TestStaleDotNetTargetRefusesValidationAndPreservesHook(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	var out, errOut bytes.Buffer
	ctx := cli.Context{Stdout: &out, Stderr: &errOut}
	if code := newInitCommand().Run(ctx, []string{"--dotnet"}); code != exitOK {
		t.Fatalf("init: %d %s", code, errOut.String())
	}
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
		t.Fatalf("hook install: %s", stderr)
	}
	hookPath := filepath.Join(root, ".git", "hooks", "pre-push")
	initial, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "services/api/Api.slnx")); err != nil {
		t.Fatal(err)
	}
	if code, report := runGeneratedValidate(""); code != exitUsage || !strings.Contains(report, "dotnet.solution") {
		t.Fatalf("missing .NET target not diagnosed: %d %q", code, report)
	}
	out.Reset()
	errOut.Reset()
	if code := runDoctor("", ctx); code != exitUsage || !strings.Contains(out.String(), "generated target:") {
		t.Fatalf("doctor omitted missing target: %d %q %q", code, out.String(), errOut.String())
	}
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy", "--force"}); code != exitUsage ||
		!strings.Contains(stderr, "invalid generated checks") {
		t.Fatalf("hook install accepted stale target: %d %s", code, stderr)
	}
	after, err := os.ReadFile(hookPath)
	if err != nil || !bytes.Equal(initial, after) {
		t.Fatalf("refused installation modified the original hook: %v", err)
	}
	if code := runSetup(false, true, false, false, "", ctx); code != exitUsage {
		t.Fatalf("setup admitted stale generated target: %d", code)
	}
	after, err = os.ReadFile(hookPath)
	if err != nil || !bytes.Equal(initial, after) {
		t.Fatalf("refused setup modified the original hook: %v", err)
	}
}

func TestDeletedRootPackageScriptRefusesHookAdmission(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "package.json", mixedManifest())
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	var out, errOut bytes.Buffer
	ctx := cli.Context{Stdout: &out, Stderr: &errOut}
	if code := newInitCommand().Run(ctx, []string{"--dotnet", "--node"}); code != exitOK {
		t.Fatalf("init: %d %s", code, errOut.String())
	}
	writeMixedFile(t, root, "package.json", `{"packageManager":"pnpm@9","scripts":{"build":"turbo run build"}}`)
	if code, report := runGeneratedValidate(""); code != exitUsage || !strings.Contains(report, "node-lint") {
		t.Fatalf("removed root script not detected: %d %s", code, report)
	}
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitUsage ||
		!strings.Contains(stderr, "node-lint") {
		t.Fatalf("broken Node hook install admitted: %d %s", code, stderr)
	}
	if _, err := os.Lstat(filepath.Join(root, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatalf("hook created despite refused admission: %v", err)
	}
}

func TestGeneratedWorkingDirectoryMissingOrSymlinked(t *testing.T) {
	for _, name := range []string{"missing", "file", "symlink"} {
		t.Run(name, func(t *testing.T) {
			if name == "symlink" && runtime.GOOS == "windows" {
				t.Skip("Windows symlinks may require developer mode")
			}
			root := withTempRepo(t)
			if name == "file" {
				writeMixedFile(t, root, "target", "content")
			}
			if name == "symlink" {
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "target")); err != nil {
					t.Fatal(err)
				}
			}
			cwd := "target"
			if name == "missing" {
				cwd = "not-created"
			}
			checks := stampGeneratedChecks([]config.Check{{Name: "lint", Run: "echo ok", Cwd: cwd}}, "template:go")
			path := config.DefaultConfigPath(root)
			if err := config.Save(path, &config.Config{Checks: checks}); err != nil {
				t.Fatal(err)
			}
			if code, report := runGeneratedValidate(""); code != exitUsage ||
				!strings.Contains(report, "working directory") {
				t.Fatalf("invalid generated cwd was accepted: %d %q", code, report)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Checks[0].Source = ""
			cfg.Checks[0].ID = ""
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			if code, report := runGeneratedValidate(""); code != exitOK {
				t.Fatalf("user-managed cwd unexpectedly refused: %d %q", code, report)
			}
			if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
				t.Fatalf("user-managed check refused by hook admission: %s", stderr)
			}
		})
	}
}

func TestGeneratedIdentityChangeRequiresOwnershipRelease(t *testing.T) {
	root := withTempRepo(t)
	checks := stampGeneratedChecks([]config.Check{{Name: "lint", Run: "echo ok"}}, "template:go")
	checks[0].Run = "echo changed"
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: checks}); err != nil {
		t.Fatal(err)
	}
	if code, report := runGeneratedValidate(""); code != exitUsage || !strings.Contains(report, "ownership") {
		t.Fatalf("modified generated ID accepted: %d %s", code, report)
	}
	checks[0].Source, checks[0].ID = "", ""
	if err := config.Save(path, &config.Config{Checks: checks}); err != nil {
		t.Fatal(err)
	}
	if code, report := runGeneratedValidate(""); code != exitOK {
		t.Fatalf("explicit user-owned check rejected: %d %s", code, report)
	}
}

func TestLegacyDotNetRootFallbackRequiresSingleTarget(t *testing.T) {
	root := withTempRepo(t)
	checks := stampGeneratedChecks([]config.Check{
		{Name: "tests", Run: "dotnet test -c Release"},
		{Name: "format", Run: "dotnet format --verify-no-changes"},
	}, "template:dotnet")
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: checks}); err != nil {
		t.Fatal(err)
	}
	if code, report := runGeneratedValidate(""); code != exitUsage || !strings.Contains(report, "root .NET command") {
		t.Fatalf("legacy root fallback was accepted: %d %q", code, report)
	}
	writeMixedFile(t, root, "App.slnx", "<Solution />")
	if code, report := runGeneratedValidate(""); code != exitOK {
		t.Fatalf("single root solution rejected: %d %q", code, report)
	}
	writeMixedFile(t, root, "Other.slnx", "<Solution />")
	if code, report := runGeneratedValidate(""); code != exitUsage || !strings.Contains(report, "root .NET command") {
		t.Fatalf("ambiguous root selection accepted: %d %q", code, report)
	}
}

func TestValidationDoesNotExecuteUserCommands(t *testing.T) {
	root := withTempRepo(t)
	marker := filepath.Join(root, "executed")
	checks := stampGeneratedChecks([]config.Check{
		{Name: "unavailable", Run: "echo should-not-run > executed", Cwd: "missing"},
	}, "template:go")
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: checks}); err != nil {
		t.Fatal(err)
	}
	if code, _ := runGeneratedValidate(""); code != exitUsage {
		t.Fatalf("invalid generated target accepted: %d", code)
	}
	var out, errOut bytes.Buffer
	_ = runDoctor("", cli.Context{Stdout: &out, Stderr: &errOut})
	_, _, _ = runHookCmd([]string{"install", "--no-copy"})
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("validation executed a configured command: %v", err)
	}
}

func TestUnusableConfigRefusesHookReplacement(t *testing.T) {
	for _, kind := range []string{"malformed", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			if kind == "dangling symlink" && runtime.GOOS == "windows" {
				t.Skip("Windows symlinks may require developer mode")
			}
			root := withTempRepo(t)
			if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
				t.Fatalf("initial hook install failed: %s", stderr)
			}
			hookPath := filepath.Join(root, ".git", "hooks", "pre-push")
			before, err := os.ReadFile(hookPath)
			if err != nil {
				t.Fatal(err)
			}
			configPath := config.DefaultConfigPath(root)
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if kind == "malformed" {
				if err := os.WriteFile(configPath, []byte("invalid yaml config"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(filepath.Join(t.TempDir(), "missing.yaml"), configPath); err != nil {
				t.Fatal(err)
			}
			if code, _, stderr := runHookCmd([]string{"install", "--no-copy", "--force"}); code != exitUsage {
				t.Fatalf("hook installation admitted unusable config: %d %s", code, stderr)
			}
			after, err := os.ReadFile(hookPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("refused installation changed hook: %v", err)
			}
		})
	}
}

func TestGeneratedTargetsRemainValidAcrossLockfileAndManagerChanges(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "package.json", mixedManifest())
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	writeMixedFile(t, root, "services/api/Api.csproj", "<Project />")
	var out, errOut bytes.Buffer
	if code := newInitCommand().Run(cli.Context{Stdout: &out, Stderr: &errOut},
		[]string{"--dotnet", "--node"}); code != exitOK {
		t.Fatalf("init: %d %s", code, errOut.String())
	}
	writeMixedFile(t, root, "services/api/packages.lock.json", "{}")
	writeMixedFile(t, root, "package.json", `{"packageManager":"npm@10","scripts":{"lint":"turbo run lint","test":"turbo run test","build":"turbo run build"}}`)
	if code, report := runGeneratedValidate(""); code != exitOK {
		t.Fatalf("valid existing commands rejected after workspace metadata changes: %d %s", code, report)
	}
}

func TestGeneratedDotNetCommandMustTargetRecordedSolution(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	writeMixedFile(t, root, "services/worker/Worker.slnx", "<Solution />")
	var out, errOut bytes.Buffer
	if code := newInitCommand().Run(cli.Context{Stdout: &out, Stderr: &errOut},
		[]string{"--dotnet", "--solution", "services/api/Api.slnx"}); code != exitOK {
		t.Fatalf("init: %d %s", code, errOut.String())
	}
	path := config.DefaultConfigPath(root)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Meta.Inputs["dotnet.solution"] = "services/worker/Worker.slnx"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	if code, report := runGeneratedValidate(""); code != exitUsage ||
		!strings.Contains(report, "does not target the recorded solution") {
		t.Fatalf("generated target mismatch accepted: %d %s", code, report)
	}
}

func TestFrontendTemplateFallbackRequiresRootScripts(t *testing.T) {
	root := withTempRepo(t)
	checks := stampGeneratedChecks([]config.Check{
		{Name: "lint", Run: "npm run lint"},
		{Name: "tests", Run: "npm test"},
		{Name: "build", Run: "npm run build"},
	}, "template:react")
	if err := config.Save(config.DefaultConfigPath(root), &config.Config{Checks: checks}); err != nil {
		t.Fatal(err)
	}
	if code, report := runGeneratedValidate(""); code != exitUsage || !strings.Contains(report, "root Node profile") {
		t.Fatalf("missing frontend root scripts accepted: %d %s", code, report)
	}
	writeMixedFile(t, root, "package.json", `{"scripts":{"lint":"eslint .","test":"vitest","build":"vite build"}}`)
	if code, report := runGeneratedValidate(""); code != exitOK {
		t.Fatalf("valid existing npm frontend template rejected: %d %s", code, report)
	}
}

func TestGeneratedWorkingDirectoryAllowsContainedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlinks may require developer mode")
	}
	root := withTempRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "src", "checks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "src"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	checks := stampGeneratedChecks([]config.Check{
		{Name: "lint", Run: "echo ok", Cwd: "linked/checks"},
	}, "template:go")
	if err := config.Save(config.DefaultConfigPath(root), &config.Config{Checks: checks}); err != nil {
		t.Fatal(err)
	}
	if code, report := runGeneratedValidate(""); code != exitOK {
		t.Fatalf("repository-contained working directory refused: %d %s", code, report)
	}
}

func TestModernDotNetProfileRequiresRecordedSolution(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	var out, errOut bytes.Buffer
	if code := newInitCommand().Run(cli.Context{Stdout: &out, Stderr: &errOut},
		[]string{"--dotnet"}); code != exitOK {
		t.Fatalf("init: %d %s", code, errOut.String())
	}
	path := config.DefaultConfigPath(root)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	delete(cfg.Meta.Inputs, "dotnet.solution")
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	if code, report := runGeneratedValidate(""); code != exitUsage ||
		!strings.Contains(report, "solution metadata is missing") {
		t.Fatalf("missing generated target metadata accepted: %d %s", code, report)
	}
}
