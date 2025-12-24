package runner

import (
	"fmt"
	"strings"
)

const headlineMaxLen = 140

// eslint prints a file path line and then a handful of issue lines.
// We only scan a short window to avoid grabbing unrelated content.
const eslintLookaheadLines = 6

type headlineRule struct {
	name      string
	extractor func(output string) (string, bool)
}

var headlineRules = []headlineRule{
	{
		name: "go test fail",
		extractor: func(output string) (string, bool) {
			match := reGoTestFail.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return "Test failed: " + match[1], true
		},
	},
	{
		name: "go test timeout",
		extractor: func(output string) (string, bool) {
			match := reGoTestTimeout.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return "Go test timeout after " + strings.TrimSpace(match[1]), true
		},
	},
	{
		name: "pytest fail",
		extractor: func(output string) (string, bool) {
			match := rePytestFail.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return "Pytest failed: " + strings.TrimSpace(match[1]), true
		},
	},
	{
		name: "jest fail",
		extractor: func(output string) (string, bool) {
			match := reJestFail.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return "Jest failed: " + strings.TrimSpace(match[1]), true
		},
	},
	{
		name: "go test package fail",
		extractor: func(output string) (string, bool) {
			match := reGoTestPkg.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return "Package failed: " + match[1], true
		},
	},
	{
		name: "dotnet fail",
		extractor: func(output string) (string, bool) {
			match := reDotnetFail.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return ".NET failed: " + strings.TrimSpace(match[1]), true
		},
	},
	{
		name: "tsc error",
		extractor: func(output string) (string, bool) {
			match := reTscError.FindStringSubmatch(output)
			if len(match) != 5 {
				return "", false
			}
			return fmt.Sprintf("%s:%s: %s", strings.TrimSpace(match[1]), match[2], strings.TrimSpace(match[4])), true
		},
	},
	{
		name: "dotnet build error",
		extractor: func(output string) (string, bool) {
			match := reDotnetBuildError.FindStringSubmatch(output)
			if len(match) != 5 {
				return "", false
			}
			return fmt.Sprintf("%s:%s: %s", strings.TrimSpace(match[1]), match[2], strings.TrimSpace(match[4])), true
		},
	},
	{
		name: "maven error",
		extractor: func(output string) (string, bool) {
			match := reMavenError.FindStringSubmatch(output)
			if len(match) != 5 {
				return "", false
			}
			return fmt.Sprintf("%s:%s: %s", strings.TrimSpace(match[1]), match[2], strings.TrimSpace(match[4])), true
		},
	},
	{
		name: "gcc error",
		extractor: func(output string) (string, bool) {
			match := reGccError.FindStringSubmatch(output)
			if len(match) != 5 {
				return "", false
			}
			return fmt.Sprintf("%s:%s: %s", strings.TrimSpace(match[1]), match[2], strings.TrimSpace(match[4])), true
		},
	},
	{
		name: "rust error",
		extractor: func(output string) (string, bool) {
			match := reRustError.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return "Rust error: " + strings.TrimSpace(match[1]), true
		},
	},
	{
		name: "black would reformat",
		extractor: func(output string) (string, bool) {
			match := reBlackFormat.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return "Black would reformat: " + strings.TrimSpace(match[1]), true
		},
	},
	{
		name: "terraform error",
		extractor: func(output string) (string, bool) {
			match := reTerraformErr.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return "Terraform error: " + strings.TrimSpace(match[1]), true
		},
	},
	{
		name: "eslint headline",
		extractor: func(output string) (string, bool) {
			headline := eslintHeadline(output)
			if strings.TrimSpace(headline) == "" {
				return "", false
			}
			return headline, true
		},
	},
	{
		name: "ruff issue",
		extractor: func(output string) (string, bool) {
			match := reRuffIssue.FindStringSubmatch(output)
			if len(match) != 6 {
				return "", false
			}
			return fmt.Sprintf(
				"%s:%s:%s: %s %s",
				strings.TrimSpace(match[1]),
				match[2],
				match[3],
				strings.TrimSpace(match[4]),
				strings.TrimSpace(match[5]),
			), true
		},
	},
	{
		name: "npm missing script",
		extractor: func(output string) (string, bool) {
			match := reNpmMissingScript.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return "npm missing script: " + strings.TrimSpace(match[1]), true
		},
	},
	{
		name: "file:line:col style issue",
		extractor: func(output string) (string, bool) {
			match := reFileLineCol.FindStringSubmatch(output)
			if len(match) != 5 {
				return "", false
			}
			return fmt.Sprintf("%s:%s: %s", strings.TrimSpace(match[1]), match[2], strings.TrimSpace(match[4])), true
		},
	},
	{
		name: "file:line style issue",
		extractor: func(output string) (string, bool) {
			match := reFileLine.FindStringSubmatch(output)
			if len(match) != 4 {
				return "", false
			}
			return fmt.Sprintf("%s:%s: %s", strings.TrimSpace(match[1]), match[2], strings.TrimSpace(match[3])), true
		},
	},
	{
		name: "first error: style issue",
		extractor: func(output string) (string, bool) {
			match := reFirstError.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return strings.TrimSpace(match[1]), true
		},
	},
	{
		name: "jest bullet",
		extractor: func(output string) (string, bool) {
			match := reJestBullet.FindStringSubmatch(output)
			if len(match) != 2 {
				return "", false
			}
			return strings.TrimSpace(match[1]), true
		},
	},
}

func ExtractHeadline(checkName string, output string) string {
	normalizedOutput := normalizeOutputNewlines(output)
	trimmedOutput := strings.TrimSpace(normalizedOutput)
	if trimmedOutput == "" {
		return ""
	}

	for _, rule := range headlineRules {
		headline, ok := rule.extractor(trimmedOutput)
		if !ok {
			continue
		}
		return trimHeadline(headline)
	}

	// Enterprise-friendly deterministic fallback: if we couldn't parse anything useful,
	// at least return a stable headline tied to the failing check.
	trimmedName := strings.TrimSpace(checkName)
	if trimmedName != "" {
		return trimHeadline("Failed: " + trimmedName)
	}

	return ""
}

func eslintHeadline(out string) string {
	lines := strings.Split(out, "\n")
	for fileLineIndex := range lines {
		fileLine := strings.TrimSpace(lines[fileLineIndex])
		if fileLine == "" {
			continue
		}
		if !reEslintFile.MatchString(fileLine) {
			continue
		}

		lookaheadLimit := fileLineIndex + eslintLookaheadLines
		if lookaheadLimit >= len(lines) {
			lookaheadLimit = len(lines) - 1
		}

		for issueLineIndex := fileLineIndex + 1; issueLineIndex <= lookaheadLimit; issueLineIndex++ {
			issueLine := strings.TrimSpace(lines[issueLineIndex])
			if issueLine == "" {
				continue
			}
			match := reEslintIssue.FindStringSubmatch(issueLine)
			if len(match) == 3 {
				return fmt.Sprintf("%s:%s: %s", fileLine, match[1], strings.TrimSpace(match[2]))
			}
		}
	}
	return ""
}

func trimHeadline(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}

	// Rune-safe truncation (don’t split UTF-8).
	runes := []rune(trimmed)
	if len(runes) <= headlineMaxLen {
		return trimmed
	}
	if headlineMaxLen <= 3 {
		return string(runes[:headlineMaxLen])
	}
	return string(runes[:headlineMaxLen-3]) + "..."
}

func normalizeOutputNewlines(output string) string {
	normalized := strings.ReplaceAll(output, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return normalized
}
