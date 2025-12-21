package runner

import (
	"runtime"
	"strings"
	"testing"

	"build-bouncer/internal/config"
)

func TestRunAllReportSkipsOSMismatch(t *testing.T) {
	root := t.TempDir()
	other := map[string]string{
		"windows": "linux",
		"linux":   "windows",
		"darwin":  "windows",
	}[runtime.GOOS]

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{
				Name: "os-only",
				Run:  "echo ok",
				OS:   config.StringList{other},
			},
		},
	}

	rep, err := RunAllReport(root, cfg, Options{})
	if err != nil {
		t.Fatalf("RunAllReport error: %v", err)
	}
	if len(rep.Failures) != 0 {
		t.Fatalf("expected no failures, got %+v", rep.Failures)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0] != "os-only" {
		t.Fatalf("expected skipped check, got %+v", rep.Skipped)
	}
	if reason := rep.SkipReasons["os-only"]; !strings.Contains(reason, "os mismatch") {
		t.Fatalf("expected os mismatch reason, got %q", reason)
	}
}

func TestRunAllReportSkipsMissingTools(t *testing.T) {
	root := t.TempDir()
	missingTool := "bb_missing_tool_12345"
	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{
				Name:     "needs-tool",
				Run:      "echo ok",
				Requires: config.StringList{missingTool},
			},
		},
	}

	rep, err := RunAllReport(root, cfg, Options{})
	if err != nil {
		t.Fatalf("RunAllReport error: %v", err)
	}
	if len(rep.Failures) != 0 {
		t.Fatalf("expected no failures, got %+v", rep.Failures)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0] != "needs-tool" {
		t.Fatalf("expected skipped check, got %+v", rep.Skipped)
	}
	if reason := rep.SkipReasons["needs-tool"]; !strings.Contains(reason, missingTool) {
		t.Fatalf("expected missing tool reason, got %q", reason)
	}
}
