package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestExplicitFailureClassification(t *testing.T) {
	cases := []struct {
		name string
		check config.Check
		expectAllowed bool
		expectUnavailable bool
	}{
		{name: "default fail closed", check: config.Check{Name: "lint", Run: "exit 7"}},
		{name: "allowed ordinary failure", check: config.Check{Name: "lint", Run: "exit 7", AllowFailure: true}, expectAllowed: true},
		{name: "unavailable required tool", check: config.Check{Name: "lint", Run: "echo never", AllowFailure: true, Requires: config.StringList{missingTestTool}}, expectUnavailable: true},
		{name: "timeout is never allowed", check: config.Check{Name: "lint", Run: sleepCommand(2), AllowFailure: true, Timeout: 100 * time.Millisecond}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, err := RunAllReport(t.TempDir(), &config.Config{Checks: []config.Check{tc.check}}, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Failures) != 1 || report.Failures[0] != tc.check.Name {
				t.Fatalf("failure was lost: %+v", report)
			}
			if report.AllowedFailures[tc.check.Name] != tc.expectAllowed {
				t.Fatalf("unexpected allowFailure classification: %+v", report)
			}
			if (len(report.Unavailable) != 0) != tc.expectUnavailable {
				t.Fatalf("unexpected unavailable classification: %+v", report)
			}
		})
	}
}

func TestLaxDoesNotAllowFailFastToSkipRemainingChecks(t *testing.T) {
	cfg := &config.Config{Checks: []config.Check{
		{Name: "optional", Run: "exit 7", AllowFailure: true},
		{Name: "required", Run: "echo must-run"},
	}}
	report, err := RunAllReport(t.TempDir(), cfg, Options{FailFast: true, MaxParallel: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Canceled) != 1 || report.Canceled[0] != "required" || !report.AllowedFailures["optional"] {
		t.Fatalf("fail fast did not record required canceled check: %+v", report)
	}
}

func TestNoExecutedChecksErrorHasStableIdentity(t *testing.T) {
	cfg := &config.Config{Checks: []config.Check{
		{Name: "optional", Run: "echo never", AllowMissingTools: true, Requires: config.StringList{missingTestTool}},
	}}
	_, err := RunAllReport(t.TempDir(), cfg, Options{Context: context.Background()})
	if !errors.Is(err, ErrNoChecksExecuted) || !strings.Contains(err.Error(), "all 1 configured checks were skipped") {
		t.Fatalf("all-skipped sentinel missing: %v", err)
	}
}
