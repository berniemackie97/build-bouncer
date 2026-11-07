package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func writeDotNetFixture(t *testing.T, root string, locked bool) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "OpenConquer.Client.slnx"), []byte("<Solution />\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(root, "src", "App")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "App.csproj"), []byte("<Project />"), 0o644); err != nil {
		t.Fatal(err)
	}
	if locked {
		if err := os.WriteFile(filepath.Join(projectDir, "packages.lock.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDotNetInitDetectsSolutionAndLockedProjects(t *testing.T) {
	root := withTempRepo(t)
	writeDotNetFixture(t, root, true)
	var stdout, stderr bytes.Buffer
	if code := newInitCommand().Run(cli.Context{Stdout: &stdout, Stderr: &stderr}, []string{"--dotnet"}); code != exitOK {
		t.Fatalf("init failed: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(config.DefaultConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Meta.Inputs["dotnet.solution"] != "OpenConquer.Client.slnx" || len(cfg.Checks) != 4 {
		t.Fatalf("solution detection failed: %+v", cfg)
	}
	want := []string{
		"dotnet restore OpenConquer.Client.slnx --locked-mode",
		"dotnet format OpenConquer.Client.slnx --verify-no-changes --no-restore",
		"dotnet build OpenConquer.Client.slnx --configuration Release --no-restore",
		"dotnet test OpenConquer.Client.slnx --configuration Release --no-build --no-restore",
	}
	for i, check := range cfg.Checks {
		if check.Run != want[i] || check.Source != "template:dotnet" || check.ID != stableCheckID(check.Source, check) {
			t.Fatalf("generated check %d differs: %+v", i, check)
		}
	}
}

func TestDotNetProfileAvoidsLockedModeWithoutCompleteLockfiles(t *testing.T) {
	root := t.TempDir()
	writeDotNetFixture(t, root, false)
	checks, _, err := dotnetProfile(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(checks[0].Run, "--locked-mode") {
		t.Fatalf("locked restore generated without lockfiles: %s", checks[0].Run)
	}
}

func TestDotNetInitDoesNotGuessAcrossMultipleSolutions(t *testing.T) {
	root := withTempRepo(t)
	writeDotNetFixture(t, root, true)
	if err := os.WriteFile(filepath.Join(root, "Other.slnx"), []byte("<Solution />"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runInit(false, false, "dotnet", cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitOK {
		t.Fatalf("init should retain generic checks on ambiguity: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(config.DefaultConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Checks) != 2 || !strings.Contains(stdout.String(), "No unique root .NET solution") {
		t.Fatalf("ambiguous solution silently selected: %+v %q", cfg.Checks, stdout.String())
	}
}

func TestConfigureUpdatesOwnedChecksAndPreservesCustomData(t *testing.T) {
	root := withTempRepo(t)
	writeDotNetFixture(t, root, true)
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runInit(false, false, "dotnet", ctx); code != exitOK {
		t.Fatalf("init failed: %d %s", code, stderr.String())
	}
	path := config.DefaultConfigPath(root)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Checks = append(cfg.Checks, config.Check{Name: "lint-extra", Run: "echo user-specified"})
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append([]byte("# custom configuration comment\n"), original...), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runConfigure(configureOptions{dotnet: true, protection: "strict", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitOK {
		t.Fatalf("configure failed: %d %s", code, stderr.String())
	}
	updated, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Protection.Level != "strict" || len(updated.Checks) != 5 || updated.Checks[4].Run != "echo user-specified" {
		t.Fatalf("custom check/protection not preserved: %+v", updated)
	}
	if updated.Checks[0].Name != "restore" || updated.Checks[3].Name != "tests" {
		t.Fatalf("unexpected check order: %+v", updated.Checks)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(after, []byte("# custom configuration comment")) {
		t.Fatal("configuration comment lost")
	}
	stdout.Reset()
	stderr.Reset()
	if code := runConfigure(configureOptions{dotnet: true, protection: "strict", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitOK {
		t.Fatalf("repeat configure failed: %d %s", code, stderr.String())
	}
	repeated, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, repeated) {
		t.Fatalf("idempotent refresh modified config: %v", err)
	}
}

func TestConfigureRefusesOverwriteOfChangedOrCustomChecks(t *testing.T) {
	for _, modified := range []bool{false, true} {
		t.Run(map[bool]string{false: "name-collision", true: "modified-generated"}[modified], func(t *testing.T) {
			root := withTempRepo(t)
			writeDotNetFixture(t, root, true)
			path := config.DefaultConfigPath(root)
			cfg := &config.Config{Checks: []config.Check{{Name: "tests", Run: "echo custom"}}}
			if modified {
				cfg.Checks = stampGeneratedChecks([]config.Check{{Name: "tests", Run: "echo original"}}, "template:dotnet")
				cfg.Checks[0].Run = "echo changed"
			}
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if code := runConfigure(configureOptions{dotnet: true, yes: true},
				cli.Context{Stdout: &stdout, Stderr: &stderr}, bufio.NewReader(strings.NewReader(""))); code != exitUsage {
				t.Fatalf("unsafe overwrite accepted: %d %s", code, stderr.String())
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("refused operation wrote configuration: %v", err)
			}
		})
	}
}

func TestConfigureCliCustomCheckAndInteractiveProtection(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: []config.Check{{Name: "tests", Run: "echo pass"}}}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runConfigure(configureOptions{check: "lint", run: "echo lint", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitOK {
		t.Fatalf("add custom check failed: %d %s", code, stderr.String())
	}
	if code := runConfigure(configureOptions{}, ctx, bufio.NewReader(strings.NewReader("2\nstrict\ny\n"))); code != exitOK {
		t.Fatalf("interactive protection failed: %d %s", code, stderr.String())
	}
	if code := runConfigure(configureOptions{remove: "lint", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitOK {
		t.Fatalf("remove custom check failed: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protection.Level != "strict" || !reflect.DeepEqual(cfg.Checks, []config.Check{{Name: "tests", Run: "echo pass"}}) {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}
}

func TestConfigureDeclineAndInvalidInputLeaveFilesUntouched(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: []config.Check{{Name: "tests", Run: "echo pass"}}}); err != nil {
		t.Fatal(err)
	}
	initial, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runConfigure(configureOptions{protection: "strict"}, ctx, bufio.NewReader(strings.NewReader("n\n"))); code != exitOK {
		t.Fatalf("decline failed: %d %s", code, stderr.String())
	}
	if code := runConfigure(configureOptions{protection: "unsafe", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitUsage {
		t.Fatal("invalid protection accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(initial, after) {
		t.Fatalf("declined/invalid update wrote file: %v", err)
	}
}
