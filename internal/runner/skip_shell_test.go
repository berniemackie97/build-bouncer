package runner

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestRequiredExecutableResolvesFromRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	executable, script := copyTestShell(t, filepath.Join(root, "tools with spaces"))
	relative := filepath.ToSlash(filepath.Join("tools with spaces", filepath.Base(executable)))
	if err := os.Mkdir(filepath.Join(root, "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	check := config.Check{
		Name: "relative requirement",
		Shell: `"` + relative + `"`,
		Run: script,
		Cwd: "work",
		Requires: config.StringList{`"` + relative + `"`},
	}
	if missing := MissingToolsAt(root, check); len(missing) != 0 {
		t.Fatalf("root-relative executable was misclassified: %v", missing)
	}
	if reason := SkipReasonAt(root, check); reason != "" {
		t.Fatalf("ready root-relative executable was skipped: %s", reason)
	}
	report, err := RunAllReport(root, &config.Config{Checks: []config.Check{check}}, Options{})
	if err != nil || len(report.Failures) != 0 || len(report.Skipped) != 0 {
		t.Fatalf("root-relative executable did not run: %+v, %v", report, err)
	}
}

func TestUnrunnableRequiredExecutableBlocks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executable admission depends on the executable format")
	}
	root := t.TempDir()
	path := filepath.Join(root, "tools", "unrunnable")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	check := config.Check{Name: "unrunnable", Run: "echo must-not-run", Requires: config.StringList{"tools/unrunnable"}}
	report, err := RunAllReport(root, &config.Config{Checks: []config.Check{check}}, Options{})
	if err != nil || len(report.Failures) != 1 || len(report.Skipped) != 0 || !strings.Contains(report.Unavailable[check.Name], "missing tools") {
		t.Fatalf("unrunnable required file was accepted: %+v, %v", report, err)
	}
}

func TestInvalidRequiredExecutableCannotBeSkipped(t *testing.T) {
	check := config.Check{Name: "invalid requires", Run: "echo must-not-run", Requires: config.StringList{"sh -c"}, AllowMissingTools: true}
	report, err := RunAllReport(t.TempDir(), &config.Config{Checks: []config.Check{check}}, Options{})
	if err != nil || len(report.Failures) != 1 || len(report.Skipped) != 0 || !strings.Contains(report.Unavailable[check.Name], "invalid required executable") {
		t.Fatalf("invalid prerequisite bypassed through optional skip: %+v, %v", report, err)
	}
}
