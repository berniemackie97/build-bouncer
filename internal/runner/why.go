package runner

import (
	"fmt"
	"strings"
)

func ExtractWhy(checkName string, output string) string {
	out := strings.ReplaceAll(output, "\r\n", "\n")
	if strings.TrimSpace(out) == "" {
		return ""
	}

	if m := reGoTestTimeout.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Go test timeout after " + strings.TrimSpace(m[1]))
	}
	if m := reGoTestFail.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Test failed: " + strings.TrimSpace(m[1]))
	}
	if m := rePytestFail.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Pytest failed: " + strings.TrimSpace(m[1]))
	}
	if m := reJestFail.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Jest failed: " + strings.TrimSpace(m[1]))
	}
	if m := reRuffIssue.FindStringSubmatch(out); len(m) == 6 {
		return trimHeadline(fmt.Sprintf("Ruff %s: %s:%s:%s: %s", strings.TrimSpace(m[4]), strings.TrimSpace(m[1]), m[2], m[3], strings.TrimSpace(m[5])))
	}
	if m := reTscError.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf("TypeScript: %s:%s:%s: %s", strings.TrimSpace(m[1]), m[2], m[3], strings.TrimSpace(m[4])))
	}
	if m := reDotnetBuildError.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf(".NET error: %s:%s:%s: %s", strings.TrimSpace(m[1]), m[2], m[3], strings.TrimSpace(m[4])))
	}
	if m := reMavenError.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf("Maven error: %s:%s:%s: %s", strings.TrimSpace(m[1]), m[2], m[3], strings.TrimSpace(m[4])))
	}
	if m := reGccError.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf("Compiler error: %s:%s:%s: %s", strings.TrimSpace(m[1]), m[2], m[3], strings.TrimSpace(m[4])))
	}
	if headline := eslintHeadline(out); headline != "" {
		return trimHeadline("ESLint: " + headline)
	}

	if m := reRustError.FindStringSubmatch(out); len(m) == 2 {
		loc := ""
		if lm := reRustLocation.FindStringSubmatch(out); len(lm) == 4 {
			loc = formatLocation(lm[1], lm[2], lm[3])
		}
		msg := "Rust error: " + strings.TrimSpace(m[1])
		if loc != "" {
			msg += " (" + loc + ")"
		}
		return trimHeadline(msg)
	}
	if lm := reRustLocation.FindStringSubmatch(out); len(lm) == 4 {
		return trimHeadline("Rust error at " + formatLocation(lm[1], lm[2], lm[3]))
	}

	if m := reBlackFormat.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Black would reformat: " + strings.TrimSpace(m[1]))
	}
	if m := reTerraformErr.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Terraform error: " + strings.TrimSpace(m[1]))
	}
	if m := reNpmMissingScript.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("npm missing script: " + strings.TrimSpace(m[1]))
	}
	if m := reFileLineCol.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf("Error at %s:%s:%s: %s", strings.TrimSpace(m[1]), m[2], m[3], strings.TrimSpace(m[4])))
	}
	if m := reFileLine.FindStringSubmatch(out); len(m) == 4 {
		return trimHeadline(fmt.Sprintf("Error at %s:%s: %s", strings.TrimSpace(m[1]), m[2], strings.TrimSpace(m[3])))
	}
	if m := reFirstError.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Error: " + strings.TrimSpace(m[1]))
	}

	return ""
}
