package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLoadDefaultsAndValidation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1

checks:
  - name: "lint"
    run: "go vet ./..."
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Insults.File != ".buildbouncer/assets/insults/default.json" {
		t.Fatalf("expected default insults file, got %q", cfg.Insults.File)
	}
	if cfg.Banter.File != ".buildbouncer/assets/banter/default.json" {
		t.Fatalf("expected default banter file, got %q", cfg.Banter.File)
	}
	if cfg.Insults.Locale != "en" || cfg.Banter.Locale != "en" {
		t.Fatalf("expected default locales to be en, got insults=%q banter=%q", cfg.Insults.Locale, cfg.Banter.Locale)
	}
	if cfg.Banter.Enabled == nil || !*cfg.Banter.Enabled {
		t.Fatalf("expected banter enabled by default")
	}
	if cfg.Version != 1 {
		t.Fatalf("expected version 1, got %d", cfg.Version)
	}
}

func TestLoadFailsWithNoChecks(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1
checks: []
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error for missing checks, got nil")
	}
}

func TestLoadFailsOnUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 2
checks:
  - name: t
    run: echo hi
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
}

func TestLoadFailsWithShellArguments(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1
checks:
  - name: bad-shell
    run: echo hi
    shell: "bash -lc"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err == nil || !strings.Contains(err.Error(), "shell") {
		t.Fatalf("expected shell validation error, got %v", err)
	}
}

func TestLoadAllowsShellPathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1
checks:
  - name: ok-shell
    run: echo hi
    shell: 'C:\Program Files\Git\bin\bash.exe'
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err != nil {
		t.Fatalf("expected shell path to be accepted, got %v", err)
	}
}

func TestLoadNormalizesOSAndRequires(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1
checks:
  - name: "lint"
    run: "go vet ./..."
    os: windows-latest
    platforms: [linux]
    requires: dotnet
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Checks[0].OS) != 2 {
		t.Fatalf("expected 2 os values, got %v", cfg.Checks[0].OS)
	}
	if len(cfg.Checks[0].Platforms) != 0 {
		t.Fatalf("expected platforms to be normalized into os, got %v", cfg.Checks[0].Platforms)
	}
	if len(cfg.Checks[0].Requires) != 1 || cfg.Checks[0].Requires[0] != "dotnet" {
		t.Fatalf("expected requires to be normalized, got %v", cfg.Checks[0].Requires)
	}
}

func TestLoadRejectsUnknownOS(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
version: 1
checks:
  - name: "lint"
    run: "go vet ./..."
    os: solaris
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(cfgPath); err == nil || !strings.Contains(err.Error(), "os") {
		t.Fatalf("expected os validation error, got %v", err)
	}
}

func TestLoadAllowMissingTools(t *testing.T) {
	cfg, err := Parse([]byte(`version: 1
checks:
  - name: optional
    run: echo ok
    allowMissingTools: true
`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Checks[0].AllowMissingTools {
		t.Fatal("explicit missing-tool policy not loaded")
	}
}

func TestRunnerLogLimitsLoad(t *testing.T) {
	cfg, err := Parse([]byte(`version: 1
checks:
  - name: test
    run: echo ok
runner:
  maxLogBytes: 4194304
  maxSavedLogs: 12
  logRetentionDays: 14
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runner.MaxLogBytes != 4194304 || cfg.Runner.MaxSavedLogs != 12 || cfg.Runner.LogRetentionDays != 14 {
		t.Fatalf("incorrect log limits: %+v", cfg.Runner)
	}

	for _, field := range []string{"maxLogBytes", "maxSavedLogs", "logRetentionDays"} {
		input := "version: 1\nchecks:\n  - name: test\n    run: echo ok\nrunner:\n  " + field + ": -1\n"
		if _, err := Parse([]byte(input)); err == nil {
			t.Fatalf("negative %s accepted", field)
		}
	}
}

func TestCanonicalOSAcceptsSupportedAliases(t *testing.T) {
	for input, want := range map[string]string{
		"windows":          "windows",
		" Windows-Latest ": "windows",
		"windows-2025":     "windows",
		"macos":            "macos",
		"darwin":           "macos",
		"OSX":              "macos",
		"macos-15":         "macos",
		"linux":            "linux",
		"ubuntu":           "linux",
		"Ubuntu-24.04":     "linux",
	} {
		if got := CanonicalOS(input); got != want {
			t.Errorf("CanonicalOS(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCanonicalOSRejectsAmbiguousValues(t *testing.T) {
	for _, input := range []string{"", "solaris", "not-windows", "pre-ubuntu", "macosmaybe", "window", "linux-gnu", "windows-", "ubuntu-"} {
		if got := CanonicalOS(input); got != "" {
			t.Errorf("CanonicalOS(%q) = %q; expected unknown", input, got)
		}
		if _, err := NormalizeOSList(StringList{input}, nil); err == nil {
			t.Errorf("NormalizeOSList accepted invalid value %q", input)
		}
	}
}

func TestNormalizeOSListPreservesOrderAndRemovesAliases(t *testing.T) {
	got, err := NormalizeOSList(StringList{"Windows-Latest", "macos"}, StringList{"windows", "darwin", "ubuntu-22.04"})
	want := StringList{"windows", "macos", "linux"}
	if err != nil || len(got) != len(want) {
		t.Fatalf("NormalizeOSList = %v, %v", got, err)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NormalizeOSList = %v; want %v", got, want)
		}
	}
}

func TestRequiredExecutablesRejectArguments(t *testing.T) {
	for _, required := range []string{"dotnet test", "sh -c", "\"unterminated"} {
		cfg := "version: 1\nchecks:\n  - name: sample\n    run: echo ok\n    requires:\n      - " + strconv.Quote(required) + "\n"
		if _, err := Parse([]byte(cfg)); err == nil || !strings.Contains(err.Error(), "requires") {
			t.Fatalf("invalid required executable %q accepted: %v", required, err)
		}
	}
}

func TestRequiredExecutablesAcceptQuotedPaths(t *testing.T) {
	cfg, err := Parse([]byte(`version: 1
checks:
  - name: sample
    run: echo ok
    requires:
      - '"./tools with spaces/check"'
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Checks[0].Requires) != 1 || cfg.Checks[0].Requires[0] != "\"./tools with spaces/check\"" {
		t.Fatalf("quoted required executable changed: %+v", cfg.Checks[0].Requires)
	}
}
