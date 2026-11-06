package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestShellRequirementWithSpacesUsesExecutableParser(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "shell with spaces", "bash.exe")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []string{"\"" + path + "\"", path} {
		check := config.Check{Name: "test", Run: "echo ok", Shell: spec}
		if missing := MissingTools(check); len(missing) != 0 {
			t.Fatalf("existing executable %q was classified missing: %q", spec, missing)
		}
		if reason := SkipReason(check); reason != "" {
			t.Fatalf("existing executable %q was skipped: %q", spec, reason)
		}
	}
}

func TestMissingShellCannotBeMarkedReady(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing binary")
	check := config.Check{Name: "test", Shell: "\"" + path + "\"", Run: "echo ok", AllowMissingTools: true}
	if missing := MissingTools(check); len(missing) != 0 {
		t.Fatalf("shell must not be counted as a missing optional tool: %q", missing)
	}
	if reason := SkipReason(check); !strings.Contains(reason, filepath.Base(path)) {
		t.Fatalf("missing executable did not fail readiness: %q", reason)
	}
	report, err := RunAllReport(t.TempDir(), &config.Config{Checks: []config.Check{check}}, Options{})
	if err != nil || len(report.Failures) != 1 || report.Failures[0] != check.Name ||
		len(report.Skipped) != 0 || !strings.Contains(report.Unavailable[check.Name], filepath.Base(path)) {
		t.Fatalf("missing shell must fail even when optional tools may be skipped: %+v, %v", report, err)
	}
}
