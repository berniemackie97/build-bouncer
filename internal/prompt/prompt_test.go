package prompt

import (
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/runner"
)

func TestShouldBlockMissingRequiredToolsInLaxMode(t *testing.T) {
	report := runner.Report{
		Failures:    []string{"lint"},
		Unavailable: map[string]string{"lint": "missing tools: go"},
	}
	if !ShouldBlock(report, "lax") {
		t.Fatal("missing required prerequisites must block at every protection level")
	}
}

func TestProtectionBlocksEveryUnclassifiedFailure(t *testing.T) {
	cases := []struct {
		name, level string
		report      runner.Report
		block       bool
	}{
		{name: "lax ordinary lint", level: "lax", report: runner.Report{Failures: []string{"lint"}}, block: true},
		{name: "lax ordinary build", level: "lax", report: runner.Report{Failures: []string{"build"}}, block: true},
		{name: "lax explicitly allowed", level: "lax", report: runner.Report{Failures: []string{"arbitrary"}, AllowedFailures: map[string]bool{"arbitrary": true}}, block: false},
		{name: "lax allowed and blocking", level: "lax", report: runner.Report{Failures: []string{"optional", "required"}, AllowedFailures: map[string]bool{"optional": true}}, block: true},
		{name: "lax missing prerequisites", level: "lax", report: runner.Report{Failures: []string{"optional"}, AllowedFailures: map[string]bool{"optional": true}, Unavailable: map[string]string{"optional": "missing tool"}}, block: true},
		{name: "lax canceled", level: "lax", report: runner.Report{Failures: []string{"optional"}, AllowedFailures: map[string]bool{"optional": true}, Canceled: []string{"required"}}, block: true},
		{name: "strict explicit allow", level: "strict", report: runner.Report{Failures: []string{"optional"}, AllowedFailures: map[string]bool{"optional": true}}, block: true},
		{name: "moderate explicit allow", level: "moderate", report: runner.Report{Failures: []string{"optional"}, AllowedFailures: map[string]bool{"optional": true}}, block: true},
		{name: "no failures", level: "strict", report: runner.Report{}, block: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldBlock(tc.report, tc.level); got != tc.block {
				t.Fatalf("ShouldBlock(%s)=%t, want %t", tc.level, got, tc.block)
			}
		})
	}
}
