package main

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestConfigureUpgradesExistingTwoCheckDotNetTemplate(t *testing.T) {
	root := withTempRepo(t)
	writeDotNetFixture(t, root, true)
	path := config.DefaultConfigPath(root)
	old := stampGeneratedChecks([]config.Check{
		{Name: "tests", Run: "dotnet test -c Release"},
		{Name: "format", Run: "dotnet format --verify-no-changes"},
	}, "template:dotnet")
	cfg := &config.Config{
		Checks:     append(old, config.Check{Name: "custom", Run: "echo keep"}),
		Protection: config.Protection{Level: "strict"},
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runConfigure(configureOptions{dotnet: true, protection: "strict", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitOK {
		t.Fatalf("profile refresh failed: exit=%d stderr=%q", code, stderr.String())
	}
	updated, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Checks) != 5 || updated.Checks[0].Name != "restore" || updated.Checks[1].Name != "format" ||
		updated.Checks[2].Name != "build" || updated.Checks[3].Name != "tests" ||
		updated.Checks[4].Name != "custom" || updated.Checks[4].Run != "echo keep" {
		t.Fatalf("profile replacement lost checks or order: %+v", updated.Checks)
	}
	if updated.Runner.MaxParallel != 1 || !updated.Runner.FailFast || updated.Protection.Level != "strict" {
		t.Fatalf("profile did not enforce sequential strict execution: runner=%+v policy=%+v", updated.Runner, updated.Protection)
	}
	for _, check := range updated.Checks[:4] {
		if check.ID == "" || check.ID != stableCheckID(check.Source, check) {
			t.Fatalf("generated identity not preserved: %+v", check)
		}
	}
}

func TestConfigureRefusesConcurrentDotNetProfile(t *testing.T) {
	root := withTempRepo(t)
	writeDotNetFixture(t, root, true)
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{
		Checks: []config.Check{{Name: "custom", Run: "echo custom"}},
		Runner: config.Runner{MaxParallel: 4},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runConfigure(configureOptions{dotnet: true, yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitUsage {
		t.Fatalf("concurrent dependent checks accepted: %d", code)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected profile modified configuration: %v", err)
	}
	stderr.Reset()
	if code := runConfigure(configureOptions{dotnet: true, parallel: "1", failFast: "true", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitOK {
		t.Fatalf("explicit sequential config rejected: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runner.MaxParallel != 1 || !cfg.Runner.FailFast || len(cfg.Checks) != 5 {
		t.Fatalf("unexpected sequential profile: %+v", cfg)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := runConfigure(configureOptions{parallel: "4", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitUsage {
		t.Fatalf("dependent check concurrency was accepted: %d", code)
	}
	afterReject, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, afterReject) {
		t.Fatalf("rejected parallel configuration was written: %v", err)
	}
}

func TestConfigureRunnerMenuAndInvalidLimits(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: []config.Check{{Name: "tests", Run: "echo pass"}}}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runConfigure(configureOptions{}, ctx, bufio.NewReader(strings.NewReader("5\n1\ntrue\ny\n"))); code != exitOK {
		t.Fatalf("interactive runner setting failed: %d %s", code, stderr.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runner.MaxParallel != 1 || !cfg.Runner.FailFast {
		t.Fatalf("runner settings not persisted: %+v", cfg.Runner)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if code := runConfigure(configureOptions{parallel: "33", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitUsage {
		t.Fatal("invalid concurrency accepted")
	}
	if code := runConfigure(configureOptions{failFast: "sometimes", yes: true}, ctx, bufio.NewReader(strings.NewReader(""))); code != exitUsage {
		t.Fatal("invalid fail-fast accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatalf("invalid runner settings changed file: %v", err)
	}
}
