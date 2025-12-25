package prompt

import (
	"fmt"
	"io"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/runner"
	"github.com/berniemackie97/build-bouncer/internal/tui"
)

// FormatPrompt creates a beautifully formatted interactive prompt
func FormatPrompt(stderr io.Writer, report runner.Report, protectionLevel string) {
	fmt.Fprintln(stderr, "")

	// Title box
	titleLines := []string{
		tui.Warning("⚠ Checks Failed"),
		"",
		fmt.Sprintf("Protection Level: %s", tui.Bold(protectionLevel)),
	}

	fmt.Fprintln(stderr, tui.DrawBox("BUILD BOUNCER", titleLines, 60))
	fmt.Fprintln(stderr, "")

	// Failure summary
	failureCount := len(report.Failures)
	if failureCount == 1 {
		fmt.Fprintln(stderr, tui.Error("  1 check failed"))
	} else {
		fmt.Fprintln(stderr, tui.Error(fmt.Sprintf("  %d checks failed", failureCount)))
	}
	fmt.Fprintln(stderr, "")

	// Categorize and display failures
	categories := categorizeFailuresDetailed(report)
	if len(categories) > 0 {
		for category, failures := range categories {
			fmt.Fprintln(stderr, tui.Bold("  "+category+":"))
			for _, name := range failures {
				fmt.Fprintln(stderr, tui.Cross(name))
			}
			fmt.Fprintln(stderr, "")
		}
	}

	// Protection level info
	fmt.Fprintln(stderr, tui.Dim("  ───────────────────────────────────────────────────────────"))
	fmt.Fprintln(stderr, "")

	switch protectionLevel {
	case "strict":
		fmt.Fprintln(stderr, tui.Warning("  Strict mode: No overrides allowed"))
		fmt.Fprintln(stderr, tui.Dim("  Fix the issues above to proceed"))

	case "lax":
		fmt.Fprintln(stderr, tui.Info("  Lax mode: Critical failures only"))
		if !hasCriticalFailures(report) {
			fmt.Fprintln(stderr, tui.Success("  No critical failures detected"))
		}

	case "moderate":
		fmt.Fprintln(stderr, tui.Info("  Moderate mode: Tests and CI enforced"))
		fmt.Fprintln(stderr, tui.Dim("  Tip: Use 'git push -o force' to skip checks"))
	}

	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, tui.Dim("  ───────────────────────────────────────────────────────────"))
	fmt.Fprintln(stderr, "")
}

// categorizeFailuresDetailed returns a map of category -> failure names
func categorizeFailuresDetailed(report runner.Report) map[string][]string {
	categories := make(map[string][]string)

	for _, name := range report.Failures {
		category := "Other"

		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "build") || strings.Contains(nameLower, "compile") {
			category = "Build Failures"
		} else if strings.Contains(nameLower, "test") {
			category = "Test Failures"
		} else if strings.Contains(nameLower, "lint") || strings.Contains(nameLower, "vet") {
			category = "Lint/Style Issues"
		} else if strings.Contains(nameLower, "ci") || strings.Contains(nameLower, "workflow") {
			category = "CI Checks"
		}

		categories[category] = append(categories[category], name)
	}

	return categories
}
