package runner

import (
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

const missingTestTool = "build_bouncer_missing_executable_987654"

func differentOS(t *testing.T) string {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		return "linux"
	case "linux", "darwin":
		return "windows"
	default:
		t.Skip("unsupported platform: " + runtime.GOOS)
		return ""
	}
}

func TestRunAllReportSkipsOSMismatch(t *testing.T) {
	cfg := &config.Config{Checks: []config.Check{
		{Name: "pass", Run: "echo ok"},
		{Name: "os-only", Run: "echo skipped", OS: config.StringList{differentOS(t)}},
	}}
	report, err := RunAllReport(t.TempDir(), cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("unexpected failures: %v", report.Failures)
	}
	if len(report.Skipped) != 1 || report.Skipped[0] != "os-only" {
		t.Fatalf("unexpected skips: %v", report.Skipped)
	}
	if reason := report.SkipReasons["os-only"]; !strings.Contains(reason, "os mismatch") {
		t.Fatalf("unexpected skip reason: %q", reason)
	}
}

func TestRunAllReportRejectsAllOSSkipped(t *testing.T) {
	cfg := &config.Config{Checks: []config.Check{
		{Name: "os-only", Run: "echo skipped", OS: config.StringList{differentOS(t)}},
	}}
	report, err := RunAllReport(t.TempDir(), cfg, Options{})
	if err == nil || !strings.Contains(err.Error(), "no checks executed") {
		t.Fatalf("expected no-applicable-check error, got report=%+v err=%v", report, err)
	}
}

func TestRunAllReportFailsOnMissingTools(t *testing.T) {
	cfg := &config.Config{Checks: []config.Check{
		{Name: "needs-tool", Run: "echo ok", Requires: config.StringList{missingTestTool}},
	}}
	report, err := RunAllReport(t.TempDir(), cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.Failures[0] != "needs-tool" {
		t.Fatalf("required unavailable check must fail: %+v", report)
	}
	if reason := report.Unavailable["needs-tool"]; !strings.Contains(reason, missingTestTool) {
		t.Fatalf("missing prerequisite reason: %q", reason)
	}
	if len(report.Skipped) != 0 {
		t.Fatalf("required check must not be skipped: %v", report.Skipped)
	}
}

func TestRunAllReportAllowsExplicitMissingToolSkip(t *testing.T) {
	cfg := &config.Config{Checks: []config.Check{
		{Name: "pass", Run: "echo ok"},
		{Name: "optional-tool", Run: "echo unused", Requires: config.StringList{missingTestTool}, AllowMissingTools: true},
	}}
	report, err := RunAllReport(t.TempDir(), cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 0 || len(report.Skipped) != 1 || report.Skipped[0] != "optional-tool" {
		t.Fatalf("unexpected outcome: %+v", report)
	}
}

func TestRunAllReportRejectsOnlyOptionalMissingTools(t *testing.T) {
	cfg := &config.Config{Checks: []config.Check{
		{Name: "optional-tool", Run: "echo unused", Requires: config.StringList{missingTestTool}, AllowMissingTools: true},
	}}
	if _, err := RunAllReport(t.TempDir(), cfg, Options{}); err == nil {
		t.Fatal("no check executed must block")
	}
}

func TestRunAllReportRejectsEmptyChecks(t *testing.T) {
	if _, err := RunAllReport(t.TempDir(), &config.Config{}, Options{}); err == nil {
		t.Fatal("empty check suite must not pass")
	}
}

func TestRunAllReportFailFastOnMissingTools(t *testing.T) {
	cfg := &config.Config{Checks: []config.Check{
		{Name: "needs-tool", Run: "echo unused", Requires: config.StringList{missingTestTool}},
		{Name: "later", Run: "echo later"},
	}}
	report, err := RunAllReport(t.TempDir(), cfg, Options{MaxParallel: 1, FailFast: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.Failures[0] != "needs-tool" {
		t.Fatalf("missing prerequisite must fail: %+v", report)
	}
	if len(report.Canceled) != 1 || report.Canceled[0] != "later" {
		t.Fatalf("fail-fast must stop dispatch: %+v", report)
	}
}

func TestRunnerRejectsInvalidOSRestrictions(t *testing.T) {
	for _, input := range []string{"not-windows", "unknown-os", ""} {
		cfg := &config.Config{Checks: []config.Check{
			{Name: "valid", Run: "echo ok"},
			{Name: "invalid", Run: "echo must-not-run", OS: config.StringList{input}},
		}}
		report, err := RunAllReport(t.TempDir(), cfg, Options{})
		if err != nil {
			t.Fatalf("RunAllReport returned unexpected error: %v", err)
		}
		if len(report.Failures) != 1 || report.Failures[0] != "invalid" {
			t.Fatalf("invalid OS restriction was not a failed check: %+v", report)
		}
		if !strings.Contains(report.Unavailable["invalid"], "invalid OS restriction") {
			t.Fatalf("missing failure reason for invalid OS: %+v", report.Unavailable)
		}
		if len(report.Skipped) != 0 {
			t.Fatalf("invalid OS restriction must not skip: %+v", report)
		}
	}
}

func TestRunnerCanonicalizesLegacyPlatforms(t *testing.T) {
	check := config.Check{OS: config.StringList{"Windows-Latest"}, Platforms: config.StringList{"darwin", "linux"}}
	applies, allowed := CheckAppliesToOS(check)
	if !applies {
		t.Fatal("supported platforms must include current platform")
	}
	if len(allowed) != 3 || allowed[0] != "windows" || allowed[1] != "macos" || allowed[2] != "linux" {
		t.Fatalf("unexpected platform normalization: %v", allowed)
	}
}
