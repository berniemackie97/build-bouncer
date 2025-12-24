package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestRunAllReportFailFastStopsDispatch(t *testing.T) {
	root := t.TempDir()

	// Runner uses sh on non-Windows when you put shell style commands in config.
	// If sh does not exist here, skip instead of inventing behavior.
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("sh"); err != nil {
			t.Skip("sh not available on this system")
		}
	}

	// Make it look like a repo so default log path goes under .git.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	markerPath := filepath.Join(root, "failfast_marker.txt")

	// First check fails. Second check would create the marker file if it ran.
	// With FailFast and MaxParallel=1, the marker check should never get dispatched.
	var failCmd, markerCmd string
	if runtime.GOOS == "windows" {
		failCmd = "echo nope & exit /b 7"
		markerCmd = `echo ran>"` + markerPath + `" & exit /b 0`
	} else {
		failCmd = "echo nope; exit 7"
		markerCmd = `echo ran > "` + markerPath + `"; exit 0`
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "fail", Run: failCmd},
			{Name: "marker", Run: markerCmd},
		},
	}

	rep, err := RunAllReport(root, cfg, Options{FailFast: true, MaxParallel: 1})
	if err != nil {
		t.Fatalf("RunAllReport error: %v", err)
	}

	if len(rep.Failures) != 1 || rep.Failures[0] != "fail" {
		t.Fatalf("expected single failure for fail, got %+v", rep.Failures)
	}

	if _, err := os.Stat(markerPath); err == nil {
		t.Fatalf("expected marker check to not run, but %q exists", markerPath)
	}
}

func TestRunAllReportRemovesLogOnSuccess(t *testing.T) {
	root := t.TempDir()

	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("sh"); err != nil {
			t.Skip("sh not available on this system")
		}
	}

	// Force default log dir under .git/build-bouncer/logs.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	var passCmd, failCmd string
	if runtime.GOOS == "windows" {
		passCmd = "echo ok & exit /b 0"
		failCmd = "echo nope & exit /b 7"
	} else {
		passCmd = "echo ok; exit 0"
		failCmd = "echo nope; exit 7"
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "pass", Run: passCmd},
			{Name: "fail", Run: failCmd},
		},
	}

	rep, err := RunAllReport(root, cfg, Options{MaxParallel: 1})
	if err != nil {
		t.Fatalf("RunAllReport error: %v", err)
	}

	if len(rep.Failures) != 1 || rep.Failures[0] != "fail" {
		t.Fatalf("expected single failure for fail, got %+v", rep.Failures)
	}

	// LogFiles is failure only by design. The fail log must exist.
	failLog := rep.LogFiles["fail"]
	if failLog == "" {
		t.Fatal("expected a log path to be recorded for fail")
	}
	if _, err := os.Stat(failLog); err != nil {
		t.Fatalf("expected fail log file to exist, stat error: %v", err)
	}

	// The pass log should have been deleted. Since we do not keep its path,
	// we validate by checking the log directory only contains the failing log.
	logDir := filepath.Join(root, ".git", "build-bouncer", "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 log file left (the failing one), got %d", len(entries))
	}

	remaining := filepath.Join(logDir, entries[0].Name())
	if remaining != failLog {
		t.Fatalf("expected remaining log %q to match fail log %q", remaining, failLog)
	}

	// Make sure success did not sneak into LogFiles.
	if got := rep.LogFiles["pass"]; got != "" {
		t.Fatalf("expected no log path recorded for pass, got %q", got)
	}
}
