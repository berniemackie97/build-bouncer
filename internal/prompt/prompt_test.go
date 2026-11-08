package prompt

import (
	"bytes"
	"strings"
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
		{name: "lax timed out", level: "lax", report: runner.Report{Failures: []string{"optional"}, AllowedFailures: map[string]bool{"optional": true}, TimedOut: []string{"optional"}}, block: true},
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

func TestInteractiveOverrideEligibilityAndInput(t *testing.T) {
	cases := []struct {
		name, level, answer      string
		report                   runner.Report
		wantOverride, wantPrompt bool
	}{
		{name: "moderate accepted", level: "moderate", answer: "yes\n", report: runner.Report{Failures: []string{"lint"}}, wantOverride: true, wantPrompt: true},
		{name: "moderate declined", level: "moderate", answer: "no\n", report: runner.Report{Failures: []string{"lint"}}, wantPrompt: true},
		{name: "lax ordinary failure", level: "lax", answer: "y\n", report: runner.Report{Failures: []string{"lint"}}, wantOverride: true, wantPrompt: true},
		{name: "strict failure", level: "strict", answer: "yes\n", report: runner.Report{Failures: []string{"lint"}}},
		{name: "no failures", level: "moderate", answer: "yes\n", report: runner.Report{}},
		{name: "unknown mode", level: "unknown", answer: "yes\n", report: runner.Report{Failures: []string{"lint"}}},
		{name: "unavailable prerequisite", level: "moderate", answer: "yes\n", report: runner.Report{Failures: []string{"lint"}, Unavailable: map[string]string{"lint": "missing tool"}}},
		{name: "canceled check", level: "moderate", answer: "yes\n", report: runner.Report{Failures: []string{"lint"}, Canceled: []string{"test"}}},
		{name: "timed out check", level: "moderate", answer: "yes\n", report: runner.Report{Failures: []string{"lint"}, TimedOut: []string{"lint"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			result, err := AskOverride(strings.NewReader(tc.answer), &output, &output, tc.report, tc.level)
			if err != nil {
				t.Fatal(err)
			}
			if result.Override != tc.wantOverride || result.Abort == tc.wantOverride {
				t.Fatalf("invalid prompt result: %+v", result)
			}
			if got := strings.Contains(output.String(), "Push anyway?"); got != tc.wantPrompt {
				t.Fatalf("prompt visible=%t want=%t: %s", got, tc.wantPrompt, output.String())
			}
			if CanOverride(tc.report, tc.level) != tc.wantPrompt {
				t.Fatal("prompt eligibility does not match actual input handling")
			}
		})
	}
}

func TestPromptFailureCategoriesAreDeterministic(t *testing.T) {
	report := runner.Report{Failures: []string{"test failure", "build failure", "lint failure"}}
	var first bytes.Buffer
	FormatPrompt(&first, report, "moderate")
	for range 10 {
		var again bytes.Buffer
		FormatPrompt(&again, report, "moderate")
		if first.String() != again.String() {
			t.Fatalf("failure category order changed: %q != %q", first.String(), again.String())
		}
	}
}
