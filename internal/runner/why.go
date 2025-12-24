package runner

import (
	"fmt"
	"strings"
)

// ExtractWhy tries to pull a single human readable reason out of a check output.
// This is meant for a short headline, not a full error report.
// Ordering matters. We prefer the most specific signals first and fall back to generic patterns last.
func ExtractWhy(checkName string, output string) string {
	normalizedOutput := normalizeNewlinesToLF(output)

	trim := strings.TrimSpace
	if trim(normalizedOutput) == "" {
		return ""
	}

	// Timeouts / test runners
	if submatches := reGoTestTimeout.FindStringSubmatch(normalizedOutput); len(submatches) == 2 {
		timeoutDetail := trim(submatches[1])
		return trimHeadline("Go test timeout after " + timeoutDetail)
	}

	if submatches := reGoTestFail.FindStringSubmatch(normalizedOutput); len(submatches) == 2 {
		failingTestName := trim(submatches[1])
		return trimHeadline("Test failed: " + failingTestName)
	}

	if submatches := rePytestFail.FindStringSubmatch(normalizedOutput); len(submatches) == 2 {
		pytestDetail := trim(submatches[1])
		return trimHeadline("Pytest failed: " + pytestDetail)
	}

	if submatches := reJestFail.FindStringSubmatch(normalizedOutput); len(submatches) == 2 {
		jestDetail := trim(submatches[1])
		return trimHeadline("Jest failed: " + jestDetail)
	}

	// Linters / compilers / build tools (prefer file/line style signals when present)
	if submatches := reRuffIssue.FindStringSubmatch(normalizedOutput); len(submatches) == 6 {
		filePath := trim(submatches[1])
		line := trim(submatches[2])
		col := trim(submatches[3])
		rule := trim(submatches[4])
		message := trim(submatches[5])
		return trimHeadline(fmt.Sprintf("Ruff %s: %s:%s:%s: %s", rule, filePath, line, col, message))
	}

	if submatches := reTscError.FindStringSubmatch(normalizedOutput); len(submatches) == 5 {
		filePath := trim(submatches[1])
		line := trim(submatches[2])
		col := trim(submatches[3])
		message := trim(submatches[4])
		return trimHeadline(fmt.Sprintf("TypeScript: %s:%s:%s: %s", filePath, line, col, message))
	}

	if submatches := reDotnetBuildError.FindStringSubmatch(normalizedOutput); len(submatches) == 5 {
		filePath := trim(submatches[1])
		line := trim(submatches[2])
		col := trim(submatches[3])
		message := trim(submatches[4])
		return trimHeadline(fmt.Sprintf(".NET error: %s:%s:%s: %s", filePath, line, col, message))
	}

	if submatches := reMavenError.FindStringSubmatch(normalizedOutput); len(submatches) == 5 {
		filePath := trim(submatches[1])
		line := trim(submatches[2])
		col := trim(submatches[3])
		message := trim(submatches[4])
		return trimHeadline(fmt.Sprintf("Maven error: %s:%s:%s: %s", filePath, line, col, message))
	}

	if submatches := reGccError.FindStringSubmatch(normalizedOutput); len(submatches) == 5 {
		filePath := trim(submatches[1])
		line := trim(submatches[2])
		col := trim(submatches[3])
		message := trim(submatches[4])
		return trimHeadline(fmt.Sprintf("Compiler error: %s:%s:%s: %s", filePath, line, col, message))
	}

	if headline := eslintHeadline(normalizedOutput); headline != "" {
		return trimHeadline("ESLint: " + headline)
	}

	// Rust: error line sometimes appears separate from location. Prefer both if possible.
	if submatches := reRustError.FindStringSubmatch(normalizedOutput); len(submatches) == 2 {
		rustMessage := trim(submatches[1])
		location := ""

		if locationMatches := reRustLocation.FindStringSubmatch(normalizedOutput); len(locationMatches) == 4 {
			location = formatLocation(locationMatches[1], locationMatches[2], locationMatches[3])
		}

		headline := "Rust error: " + rustMessage
		if location != "" {
			headline += " (" + location + ")"
		}

		return trimHeadline(headline)
	}

	if locationMatches := reRustLocation.FindStringSubmatch(normalizedOutput); len(locationMatches) == 4 {
		location := formatLocation(locationMatches[1], locationMatches[2], locationMatches[3])
		return trimHeadline("Rust error at " + location)
	}

	// Formatters / misc tooling
	if submatches := reBlackFormat.FindStringSubmatch(normalizedOutput); len(submatches) == 2 {
		filePath := trim(submatches[1])
		return trimHeadline("Black would reformat: " + filePath)
	}

	if submatches := reTerraformErr.FindStringSubmatch(normalizedOutput); len(submatches) == 2 {
		terraformMessage := trim(submatches[1])
		return trimHeadline("Terraform error: " + terraformMessage)
	}

	if submatches := reNpmMissingScript.FindStringSubmatch(normalizedOutput); len(submatches) == 2 {
		scriptName := trim(submatches[1])
		return trimHeadline("npm missing script: " + scriptName)
	}

	// Generic file:line:col patterns
	if submatches := reFileLineCol.FindStringSubmatch(normalizedOutput); len(submatches) == 5 {
		filePath := trim(submatches[1])
		line := trim(submatches[2])
		col := trim(submatches[3])
		message := trim(submatches[4])
		return trimHeadline(fmt.Sprintf("Error at %s:%s:%s: %s", filePath, line, col, message))
	}

	if submatches := reFileLine.FindStringSubmatch(normalizedOutput); len(submatches) == 4 {
		filePath := trim(submatches[1])
		line := trim(submatches[2])
		message := trim(submatches[3])
		return trimHeadline(fmt.Sprintf("Error at %s:%s: %s", filePath, line, message))
	}

	// Last resort: first "error:" style line
	if submatches := reFirstError.FindStringSubmatch(normalizedOutput); len(submatches) == 2 {
		errorMessage := trim(submatches[1])
		return trimHeadline("Error: " + errorMessage)
	}

	_ = checkName // reserved for future check-specific heuristics
	return ""
}
