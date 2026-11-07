package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func writeWorkspaceFile(t *testing.T, root, relative string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<Solution />\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDotNetInitDiscoversNestedSolutionWithoutRootFallback(t *testing.T) {
	root := withTempRepo(t)
	writeWorkspaceFile(t, root, "services/api/Api.slnx")
	writeWorkspaceFile(t, root, "services/api/Api.csproj")
	writeWorkspaceFile(t, root, "node_modules/pkg/Ignored.sln")
	writeWorkspaceFile(t, root, "services/api/bin/Ignored.slnx")
	writeWorkspaceFile(t, root, "dist/Ignored.sln")
	var stdout, stderr bytes.Buffer
	code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr}, []string{"--dotnet"})
	if code != exitOK {
		t.Fatalf("nested solution init failed: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(config.DefaultConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Meta.Inputs["dotnet.solution"] != "services/api/Api.slnx" || len(cfg.Checks) != 4 ||
		cfg.Runner.MaxParallel != 1 || !cfg.Runner.FailFast {
		t.Fatalf("wrong nested profile: %+v", cfg)
	}
	for _, check := range cfg.Checks {
		if !strings.Contains(check.Run, "services/api/Api.slnx") || check.Source != "template:dotnet" {
			t.Fatalf("root fallback or unowned check: %+v", check)
		}
	}
}

func TestDotNetInitRejectsSolutionWithoutDotNetTemplate(t *testing.T) {
	for _, args := range [][]string{
		{"--solution", "Api.slnx"},
		{"--go", "--solution", "Api.slnx"},
	} {
		root := withTempRepo(t)
		var stdout, stderr bytes.Buffer
		if code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr}, args); code != exitUsage {
			t.Fatalf("invalid option combination accepted: %v (exit %d)", args, code)
		}
		if !strings.Contains(stderr.String(), "--solution requires --dotnet") {
			t.Fatalf("wrong invalid selection error: %q", stderr.String())
		}
		if _, err := os.Lstat(config.ConfigDir(root)); !os.IsNotExist(err) {
			t.Fatalf("invalid selection created managed files: %v", err)
		}
	}
}

func TestDotNetInitFailsWithoutRunnableSolution(t *testing.T) {
	for _, name := range []string{"no-dotnet", "project-without-solution", "multiple-solutions"} {
		t.Run(name, func(t *testing.T) {
			root := withTempRepo(t)
			switch name {
			case "project-without-solution":
				writeWorkspaceFile(t, root, "src/App/App.csproj")
			case "multiple-solutions":
				writeWorkspaceFile(t, root, "services/api/Api.slnx")
				writeWorkspaceFile(t, root, "services/worker/Worker.sln")
			}
			var stdout, stderr bytes.Buffer
			ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
			if code := newInitCommand().Run(ctx, []string{"--dotnet"}); code != exitUsage {
				t.Fatalf("unsafe initialization succeeded: %d %s", code, stdout.String())
			}
			if _, err := os.Lstat(config.ConfigDir(root)); !os.IsNotExist(err) {
				t.Fatalf("failed initialization created managed files: %v", err)
			}
			if !strings.Contains(stderr.String(), "solution") && !strings.Contains(stderr.String(), ".sln") {
				t.Fatalf("error did not explain selection: %q", stderr.String())
			}
		})
	}
}

func TestDotNetInitExplicitNestedSelection(t *testing.T) {
	root := withTempRepo(t)
	writeWorkspaceFile(t, root, "services/api/My Solution.slnx")
	writeWorkspaceFile(t, root, "services/worker/Worker.slnx")
	writeWorkspaceFile(t, root, "services/api/App.csproj")
	writeWorkspaceFile(t, root, "services/api/packages.lock.json")
	var stdout, stderr bytes.Buffer
	if code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr}, []string{
		"--dotnet", "--solution", "services\\api\\My Solution.slnx",
	}); code != exitOK {
		t.Fatalf("explicit solution rejected: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(config.DefaultConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Meta.Inputs["dotnet.solution"] != "services/api/My Solution.slnx" ||
		cfg.Checks[0].Run != "dotnet restore \"services/api/My Solution.slnx\" --locked-mode" {
		t.Fatalf("selected path or quoting differs: %+v", cfg)
	}
}

func TestDotNetExplicitSolutionRefusesEscapesAndGeneratedPaths(t *testing.T) {
	root := withTempRepo(t)
	writeWorkspaceFile(t, root, "services/api/Api.slnx")
	writeWorkspaceFile(t, root, "bin/Excluded.slnx")
	for _, selected := range []string{
		"../Api.slnx", "/tmp/Api.slnx", "C:/Api.slnx", "services/../services/api/Api.slnx",
		"services//api/Api.slnx", "bin/Excluded.slnx", "services/api/Other.slnx",
	} {
		if _, _, err := dotnetProfile(root, selected); err == nil {
			t.Fatalf("unsafe selection %q accepted", selected)
		}
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require Windows developer mode")
	}
	outside := t.TempDir()
	writeWorkspaceFile(t, outside, "Outside.slnx")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := dotnetProfile(root, "linked/Outside.slnx"); err == nil {
		t.Fatal("outside solution was accepted through a symlink")
	}
	if err := os.Symlink(filepath.Join(root, "services", "api", "Api.slnx"), filepath.Join(root, "Shortcut.slnx")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := dotnetProfile(root, "Shortcut.slnx"); err == nil {
		t.Fatal("symlinked solution was accepted")
	}
}

func TestDotNetSetupFailsWithoutWritingHook(t *testing.T) {
	root := withTempRepo(t)
	writeWorkspaceFile(t, root, "services/a/A.slnx")
	writeWorkspaceFile(t, root, "services/b/B.slnx")
	var stdout, stderr bytes.Buffer
	if code := newSetupCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr},
		[]string{"--dotnet", "--no-copy"}); code != exitUsage {
		t.Fatalf("setup should fail on ambiguous target: %d %s", code, stdout.String())
	}
	if _, err := os.Lstat(config.ConfigDir(root)); !os.IsNotExist(err) {
		t.Fatalf("setup wrote config/assets: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatalf("setup installed a broken hook: %v", err)
	}
}

func TestProtectionUpdateIndependentOfFailedDotNetRefresh(t *testing.T) {
	root := withTempRepo(t)
	cfgPath := config.DefaultConfigPath(root)
	initial := &config.Config{Checks: []config.Check{{Name: "custom", Run: "echo ok"}}}
	if err := config.Save(cfgPath, initial); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runConfigure(configureOptions{dotnet: true, protection: "strict", yes: true}, ctx,
		bufio.NewReader(strings.NewReader(""))); code != exitUsage {
		t.Fatalf("missing .NET target unexpectedly accepted: %d", code)
	}
	current, err := os.ReadFile(cfgPath)
	if err != nil || !bytes.Equal(before, current) {
		t.Fatalf("failed refresh changed configuration: %v", err)
	}
	if code := runConfigure(configureOptions{protection: "strict", yes: true}, ctx,
		bufio.NewReader(strings.NewReader(""))); code != exitOK {
		t.Fatalf("independent protection change failed: %d %s", code, stderr.String())
	}
	updated, err := config.Load(cfgPath)
	if err != nil || updated.Protection.Level != "strict" || len(updated.Checks) != 1 || updated.Checks[0].Run != "echo ok" {
		t.Fatalf("protection change damaged checks: %+v, %v", updated, err)
	}
}
