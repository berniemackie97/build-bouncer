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

func writeMixedFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mixedManifest() string {
	return `{
  "packageManager": "pnpm@9.0.0",
  "scripts": {
    "lint": "turbo run lint",
    "test": "turbo run test",
    "build": "turbo run build",
    "release": "echo not-selected"
  }
}`
}

func TestMixedInitSelectsRootScriptsAndNestedSolution(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "package.json", mixedManifest())
	writeMixedFile(t, root, "pnpm-workspace.yaml", "packages:\n  - packages/*\n")
	writeMixedFile(t, root, "turbo.json", "{}")
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	writeMixedFile(t, root, "services/api/Api.csproj", "<Project />")
	writeMixedFile(t, root, "apps/mobile/package.json", `{"scripts":{"lint":"echo standalone"}}`)
	writeMixedFile(t, root, "node_modules/ignored/Other.slnx", "<Solution />")
	var stdout, stderr bytes.Buffer
	if code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr},
		[]string{"--dotnet", "--node", "--solution", "services/api/Api.slnx"}); code != exitOK {
		t.Fatalf("mixed init failed: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(config.DefaultConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Meta.Template.ID != "dotnet+node" ||
		cfg.Meta.Inputs["dotnet.solution"] != "services/api/Api.slnx" ||
		cfg.Meta.Inputs["node.runner"] != "pnpm" ||
		cfg.Meta.Inputs["node.scripts"] != "lint,test,build" ||
		cfg.Meta.Inputs["node.scope"] != "root" ||
		cfg.Meta.Inputs["node.workspace"] != "pnpm" ||
		cfg.Meta.Inputs["node.turbo"] != "present" {
		t.Fatalf("wrong mixed profile inputs: %+v", cfg.Meta)
	}
	if len(cfg.Checks) != 7 || cfg.Runner.MaxParallel != 1 || !cfg.Runner.FailFast {
		t.Fatalf("mixed checks or sequential settings incorrect: %+v", cfg)
	}
	for i, name := range []string{"restore", "format", "build", "tests", "node-lint", "node-tests", "node-build"} {
		check := cfg.Checks[i]
		source := "template:dotnet"
		if i >= 4 {
			source = "detector:node-scripts"
		}
		if check.Name != name || check.Source != source || check.ID != stableCheckID(source, check) {
			t.Fatalf("check %d incorrect: %+v", i, check)
		}
		if strings.Contains(check.Run, "apps/mobile") || strings.Contains(check.Run, "node_modules") {
			t.Fatalf("script from an unrelated package inferred: %+v", check)
		}
	}
	for i, want := range []string{"pnpm run lint", "pnpm run test", "pnpm run build"} {
		if cfg.Checks[i+4].Run != want {
			t.Fatalf("root command differs: %s, want %s", cfg.Checks[i+4].Run, want)
		}
	}
}

func TestMixedInitFailsClosedOnAmbiguityAndInvalidRootPackage(t *testing.T) {
	tests := []struct {
		name      string
		manifest  string
		solutions []string
	}{
		{"missing package", "", []string{"services/api/Api.slnx"}},
		{"invalid package", "{broken", []string{"services/api/Api.slnx"}},
		{"no selected scripts", `{"scripts":{"publish":"echo publish"}}`, []string{"services/api/Api.slnx"}},
		{"multiple solutions", mixedManifest(), []string{"services/api/Api.slnx", "services/worker/Worker.slnx"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := withTempRepo(t)
			if tc.manifest != "" {
				writeMixedFile(t, root, "package.json", tc.manifest)
			}
			for _, file := range tc.solutions {
				writeMixedFile(t, root, file, "<Solution />")
			}
			var stdout, stderr bytes.Buffer
			if code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr},
				[]string{"--node", "--dotnet"}); code != exitUsage {
				t.Fatalf("invalid mixed init accepted: %d %s", code, stdout.String())
			}
			if _, err := os.Lstat(config.ConfigDir(root)); !os.IsNotExist(err) {
				t.Fatalf("refused init wrote files: %v", err)
			}
		})
	}
}

func TestMixedInitRejectsSymlinkedRootPackage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation may require developer mode")
	}
	root := withTempRepo(t)
	outside := t.TempDir()
	writeMixedFile(t, outside, "package.json", mixedManifest())
	if err := os.Symlink(filepath.Join(outside, "package.json"), filepath.Join(root, "package.json")); err != nil {
		t.Fatal(err)
	}
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	var stdout, stderr bytes.Buffer
	if code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr},
		[]string{"--dotnet", "--node"}); code != exitUsage {
		t.Fatalf("symlinked package accepted: %d", code)
	}
	if _, err := os.Lstat(config.ConfigDir(root)); !os.IsNotExist(err) {
		t.Fatalf("refused init wrote files: %v", err)
	}
}

func TestMixedConfigureRefreshPreservesCustomChecks(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "package.json", mixedManifest())
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := newInitCommand().Run(ctx, []string{"--dotnet", "--node"}); code != exitOK {
		t.Fatalf("init failed: %d %s", code, stderr.String())
	}
	path := config.DefaultConfigPath(root)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Checks = append(cfg.Checks, config.Check{Name: "custom", Run: "echo keep"})
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	writeMixedFile(t, root, "package.json", `{"packageManager":"pnpm@9","scripts":{"lint":"turbo lint","typecheck":"tsc -b"}}`)
	opts := configureOptions{node: true, dotnet: true, yes: true}
	if code := runConfigure(opts, ctx, bufio.NewReader(strings.NewReader(""))); code != exitOK {
		t.Fatalf("combined refresh failed: %d %s", code, stderr.String())
	}
	updated, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Checks) != 7 || updated.Checks[4].Name != "node-lint" ||
		updated.Checks[5].Name != "node-typecheck" || updated.Checks[6].Name != "custom" ||
		updated.Checks[6].Run != "echo keep" || updated.Meta.Inputs["node.scripts"] != "lint,typecheck" {
		t.Fatalf("refresh altered unrelated checks: %+v", updated)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if code := runConfigure(opts, ctx, bufio.NewReader(strings.NewReader(""))); code != exitOK {
		t.Fatalf("repeat refresh failed: %d %s", code, stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatalf("idempotent refresh modified file: %v", err)
	}
}

func TestMixedConfigureRefusesModifiedGeneratedChecks(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "package.json", mixedManifest())
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := newInitCommand().Run(ctx, []string{"--node", "--dotnet"}); code != exitOK {
		t.Fatalf("init failed: %d %s", code, stderr.String())
	}
	path := config.DefaultConfigPath(root)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Checks[4].Run = "echo user-modified"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if code := runConfigure(configureOptions{node: true, yes: true}, ctx,
		bufio.NewReader(strings.NewReader(""))); code != exitUsage {
		t.Fatalf("modified generated check overwritten: exit %d", code)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("refusal modified configuration: %v", err)
	}
}

func TestMixedSetupFailureDoesNotInstallHook(t *testing.T) {
	root := withTempRepo(t)
	writeMixedFile(t, root, "package.json", mixedManifest())
	writeMixedFile(t, root, "services/api/Api.slnx", "<Solution />")
	writeMixedFile(t, root, "services/worker/Worker.slnx", "<Solution />")
	var stdout, stderr bytes.Buffer
	if code := newSetupCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr},
		[]string{"--dotnet", "--node", "--no-copy"}); code != exitUsage {
		t.Fatalf("ambiguous setup unexpectedly accepted: %d", code)
	}
	if _, err := os.Lstat(config.ConfigDir(root)); !os.IsNotExist(err) {
		t.Fatalf("ambiguous setup wrote configuration: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatalf("ambiguous setup installed hook: %v", err)
	}
}

func TestMixedOptionConflicts(t *testing.T) {
	for _, args := range [][]string{
		{"--node", "--go"},
		{"--node", "--solution", "services/api/Api.slnx"},
		{"--dotnet", "--node", "--go"},
	} {
		root := withTempRepo(t)
		var stdout, stderr bytes.Buffer
		if code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr}, args); code != exitUsage {
			t.Fatalf("invalid selection %v accepted: %d", args, code)
		}
		if _, err := os.Lstat(config.ConfigDir(root)); !os.IsNotExist(err) {
			t.Fatalf("invalid selection %v wrote files: %v", args, err)
		}
	}
}
