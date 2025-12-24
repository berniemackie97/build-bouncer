package runner

import (
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestRunAllReportSkipsOSMismatch(t *testing.T) {
	root := t.TempDir()

	// This test is written for the common GOOS values we actually support in config.
	// If we run on something else, skip instead of quietly changing what we are testing.
	var other string
	switch runtime.GOOS {
	case "windows":
		other = "linux"
	case "linux":
		other = "windows"
	case "darwin":
		other = "windows"
	default:
		t.Skip("unsupported GOOS for this test: " + runtime.GOOS)
	}

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
