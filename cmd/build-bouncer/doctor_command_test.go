package main

import (
	"bytes"
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
		!strings.Contains(stdout.String(), "executable:") || !strings.Contains(stdout.String(), "unavailable") {
		t.Fatalf("doctor did not report shell selection and failure: exit=%d, out=%q, err=%q", code, stdout.String(), stderr.String())
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
