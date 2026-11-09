package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestFailedCheckStartPreservesFailureAndCanceledPeers(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Checks: []config.Check{
		{Name: "missing-working-directory", Run: "echo must-not-start", Cwd: "missing-workdir"},
		{Name: "not-started", Run: "echo must-not-start"},
	}}
	report, err := RunAllReport(root, cfg, Options{MaxParallel: 1, FailFast: true})
	if !errors.Is(err, ErrCheckExecution) || !strings.Contains(err.Error(), "missing-working-directory") {
		t.Fatalf("startup error lost its type and identity: report=%+v err=%v", report, err)
	}
	if len(report.Failures) != 1 || report.Failures[0] != "missing-working-directory" {
		t.Fatalf("failed check disappeared from report: %+v", report)
	}
	if len(report.Canceled) != 1 || report.Canceled[0] != "not-started" {
		t.Fatalf("canceled peer missing from report: %+v", report)
	}
	if !strings.Contains(report.Unavailable["missing-working-directory"], "missing-workdir") ||
		!strings.Contains(report.FailureHeadlines["missing-working-directory"], "missing-workdir") {
		t.Fatalf("missing actionable execution error: %+v", report)
	}
	if len(report.LogFiles) != 0 {
		t.Fatalf("failed process start invented a retained log: %+v", report.LogFiles)
	}
	if _, statErr := os.Stat(filepath.Join(root, "missing-workdir")); !os.IsNotExist(statErr) {
		t.Fatalf("check created nonexistent working directory: %v", statErr)
	}
}

func TestFailedCheckStartPreservesCompletedFailureEvidence(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Checks: []config.Check{
		{Name: "failed-check", Run: failCommand("first-failure")},
		{Name: "bad-working-directory", Run: "echo must-not-start", Cwd: "missing-workdir"},
	}}
	report, err := RunAllReport(root, cfg, Options{MaxParallel: 1})
	if !errors.Is(err, ErrCheckExecution) {
		t.Fatalf("expected check execution error: %v", err)
	}
	if len(report.Failures) != 2 || report.Failures[0] != "failed-check" || report.Failures[1] != "bad-working-directory" {
		t.Fatalf("failed/completed checks were lost: %+v", report)
	}
	if !strings.Contains(report.FailureTails["failed-check"], "first-failure") {
		t.Fatalf("completed check output missing: %+v", report.FailureTails)
	}
	logPath := report.LogFiles["failed-check"]
	if !strings.HasSuffix(logPath, ".log") {
		t.Fatalf("completed check log was discarded: %+v", report.LogFiles)
	}
	logData, readErr := os.ReadFile(logPath)
	if readErr != nil || !strings.Contains(string(logData), "first-failure") {
		t.Fatalf("retained evidence is not readable: %q %v", logData, readErr)
	}
	if report.LogFiles["bad-working-directory"] != "" {
		t.Fatalf("never-started check has invented log: %+v", report.LogFiles)
	}
	if !strings.Contains(report.Unavailable["bad-working-directory"], "missing-workdir") {
		t.Fatalf("missing launch diagnosis: %+v", report.Unavailable)
	}
}

func TestOrdinaryCheckFailureStillReturnsOutcomeWithoutExecutionError(t *testing.T) {
	report, err := RunAllReport(t.TempDir(), &config.Config{Checks: []config.Check{
		{Name: "failed-check", Run: failCommand("ordinary-failure")},
	}}, Options{})
	if err != nil || len(report.Failures) != 1 || len(report.Unavailable) != 0 {
		t.Fatalf("ordinary process exit misclassified as startup error: %+v %v", report, err)
	}
}

func TestCheckWorkingDirectoryMustBeDirectory(t *testing.T) {
	root := t.TempDir()
	workingPath := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(workingPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := RunAllReport(root, &config.Config{Checks: []config.Check{{
		Name: "bad-working-directory", Run: "echo never", Cwd: "not-a-directory",
	}}}, Options{})
	if !errors.Is(err, ErrCheckExecution) ||
		!strings.Contains(err.Error(), "not-a-directory") ||
		!strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("non-directory path was not rejected: report=%+v err=%v", report, err)
	}
	if len(report.Failures) != 1 || report.Failures[0] != "bad-working-directory" ||
		!strings.Contains(report.Unavailable["bad-working-directory"], "not-a-directory") ||
		len(report.LogFiles) != 0 {
		t.Fatalf("non-directory check failure lost or misclassified: %+v", report)
	}
}

func TestParallelCheckStartFailurePreservesEveryOutcome(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Checks: []config.Check{
		{Name: "bad-working-directory", Run: "echo never", Cwd: "missing-workdir"},
		{Name: "in-flight", Run: failCommand("concurrent-failure")},
	}}
	report, err := RunAllReport(root, cfg, Options{MaxParallel: 2})
	if !errors.Is(err, ErrCheckExecution) {
		t.Fatalf("parallel start failure was not classified: %+v %v", report, err)
	}
	if !strings.Contains(report.Unavailable["bad-working-directory"], "missing-workdir") {
		t.Fatalf("startup failure disappeared: %+v", report)
	}
	outcomes := make(map[string]int, len(cfg.Checks))
	for _, name := range report.Failures {
		outcomes[name]++
	}
	for _, name := range report.Canceled {
		outcomes[name]++
	}
	for _, name := range report.Skipped {
		outcomes[name]++
	}
	for _, check := range cfg.Checks {
		if outcomes[check.Name] != 1 {
			t.Fatalf("parallel check %q had %d terminal outcomes: %+v", check.Name, outcomes[check.Name], report)
		}
	}
	if len(report.Skipped) != 0 || report.LogFiles["bad-working-directory"] != "" {
		t.Fatalf("parallel start failure skipped or fabricated a log: %+v", report)
	}
	for _, name := range report.Failures {
		if name == "in-flight" && !strings.Contains(report.FailureTails[name], "concurrent-failure") {
			t.Fatalf("finished in-flight failure lost its output: %+v", report)
		}
	}
}
