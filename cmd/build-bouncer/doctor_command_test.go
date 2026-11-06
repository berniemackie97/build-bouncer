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

func TestDoctorReportsExecutableAndMissingShell(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	missing := filepath.Join(root, "missing-shell")
	cfg := &config.Config{
		Checks: []config.Check{
			{Name: "available", Run: "echo ok"},
			{Name: "missing", Run: "echo ok", Shell: missing},
		},
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runDoctor(path, cli.Context{Stdout: &stdout, Stderr: &stderr})
	if code != exitUsage || !strings.Contains(stdout.String(), "shell error:") ||
		!strings.Contains(stdout.String(), "executable:") || !strings.Contains(stdout.String(), "unavailable") ||
		strings.Contains(stdout.String(), "skip:") {
		t.Fatalf("doctor did not report shell selection and failure: exit=%d, out=%q, err=%q", code, stdout.String(), stderr.String())
	}
}

func TestDoctorResolvesShellRelativeToRepository(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, config.ConfigDirName, config.ConfigFileName)
	exe := "sh"
	if runtime.GOOS == "windows" {
		exe = "cmd.exe"
	}
	absolute := filepath.Join(root, "tools", exe)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte("test executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Checks: []config.Check{{Name: "relative", Run: "echo ok", Shell: filepath.Join("tools", exe)}}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runDoctor(configPath, cli.Context{Stdout: &out, Stderr: &errOut}); code != exitOK ||
		!strings.Contains(out.String(), absolute) {
		t.Fatalf("doctor did not resolve shell from repository: code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestDoctorDoesNotRequireForeignOSShell(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	other := "windows"
	if runtime.GOOS == "windows" {
		other = "linux"
	}
	cfg := &config.Config{Checks: []config.Check{{
		Name:  "foreign",
		Run:   "echo ok",
		Shell: filepath.Join(root, "missing-shell"),
		OS:    config.StringList{other},
	}}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runDoctor(path, cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitOK ||
		!strings.Contains(stdout.String(), "skip:") || strings.Contains(stdout.String(), "shell error:") {
		t.Fatalf("doctor should diagnose foreign OS without requiring its shell: %d, %q, %q", code, stdout.String(), stderr.String())
	}
}

func TestDoctorResolvesRequiredToolsAtRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, config.ConfigDirName, config.ConfigFileName)
	executableName := "check"
	if runtime.GOOS == "windows" {
		executableName = "check.exe"
	}
	toolPath := filepath.Join(root, "tools", executableName)
	if err := os.MkdirAll(filepath.Dir(toolPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(toolPath, []byte("placeholder"), 0o700); err != nil {
		t.Fatal(err)
	}
	relative := filepath.ToSlash(filepath.Join("tools", executableName))
	cfg := &config.Config{Checks: []config.Check{{Name: "required", Run: "echo ok", Requires: config.StringList{relative}}}}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runDoctor(cfgPath, cli.Context{Stdout: &out, Stderr: &errOut}); code != exitOK || strings.Contains(out.String(), "missing:") {
		t.Fatalf("doctor did not resolve root-relative required executable: %d, %q, %q", code, out.String(), errOut.String())
	}
	cfg.Checks[0].Requires = config.StringList{"tools/missing-command"}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := runDoctor(cfgPath, cli.Context{Stdout: &out, Stderr: &errOut}); code != exitUsage ||
		!strings.Contains(out.String(), "missing: tools/missing-command") {
		t.Fatalf("doctor did not report missing required executable: %d, %q, %q", code, out.String(), errOut.String())
	}
}
