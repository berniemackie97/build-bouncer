package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/runner"
)

// Result represents the user's response to a prompt
type Result struct {
	Override bool // Whether user wants to override and push anyway
	Abort    bool // Whether user wants to abort
}

// AskOverride displays an interactive prompt asking if the user wants to push despite failures.
// Returns true if user wants to override (push anyway), false otherwise.
func AskOverride(stdin io.Reader, stdout, stderr io.Writer, report runner.Report, protectionLevel string) (Result, error) {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	// Protection-level-specific handling
	if protectionLevel == "strict" {
		FormatPrompt(stderr, report, protectionLevel)
		return Result{Override: false, Abort: true}, nil
	}

	// Display beautifully formatted prompt
	FormatPrompt(stderr, report, protectionLevel)

	// Prompt for input
	fmt.Fprint(stderr, "  Push anyway? [y/N]: ")

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		return Result{Override: false, Abort: true}, scanner.Err()
	}

	response := strings.ToLower(strings.TrimSpace(scanner.Text()))
	override := response == "y" || response == "yes"

	return Result{Override: override, Abort: !override}, nil
}

// categorizeFailures groups failures by type for better UX
func categorizeFailures(report runner.Report) map[string]int {
	categories := make(map[string]int)

	for _, name := range report.Failures {
		category := "Other"

		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "build") || strings.Contains(nameLower, "compile") {
			category = "Build failures"
		} else if strings.Contains(nameLower, "test") {
			category = "Test failures"
		} else if strings.Contains(nameLower, "lint") || strings.Contains(nameLower, "vet") {
			category = "Lint/style issues"
		} else if strings.Contains(nameLower, "ci") || strings.Contains(nameLower, "workflow") {
			category = "CI checks"
		}

		categories[category]++
	}

	return categories
}

func hasBlockingFailures(report runner.Report) bool {
	if len(report.Unavailable) > 0 || len(report.Canceled) > 0 {
		return true
	}
	for _, name := range report.Failures {
		if !report.AllowedFailures[name] {
			return true
		}
	}
	return false
}

func ShouldBlock(report runner.Report, protectionLevel string) bool {
	if len(report.Unavailable) > 0 || len(report.Canceled) > 0 {
		return true
	}
	if protectionLevel == "lax" {
		return hasBlockingFailures(report)
	}
	return len(report.Failures) > 0
}
