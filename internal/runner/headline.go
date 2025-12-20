package runner

import (
	"fmt"
	"strings"
)

const headlineMaxLen = 140

func ExtractHeadline(checkName string, output string) string {
	out := strings.ReplaceAll(output, "\r\n", "\n")
	if strings.TrimSpace(out) == "" {
		return ""
	}

	if m := reGoTestFail.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Test failed: " + m[1])
	}
	if m := reGoTestTimeout.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Go test timeout after " + strings.TrimSpace(m[1]))
	}
	if m := rePytestFail.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Pytest failed: " + strings.TrimSpace(m[1]))
	}
	if m := reJestFail.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Jest failed: " + strings.TrimSpace(m[1]))
	}
	if m := reGoTestPkg.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Package failed: " + m[1])
	}
	if m := reDotnetFail.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline(".NET failed: " + strings.TrimSpace(m[1]))
	}
	if m := reTscError.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf("%s:%s: %s", strings.TrimSpace(m[1]), m[2], strings.TrimSpace(m[4])))
	}
	if m := reDotnetBuildError.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf("%s:%s: %s", strings.TrimSpace(m[1]), m[2], strings.TrimSpace(m[4])))
	}
	if m := reMavenError.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf("%s:%s: %s", strings.TrimSpace(m[1]), m[2], strings.TrimSpace(m[4])))
	}
	if m := reGccError.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf("%s:%s: %s", strings.TrimSpace(m[1]), m[2], strings.TrimSpace(m[4])))
	}
	if m := reRustError.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Rust error: " + strings.TrimSpace(m[1]))
	}
	if m := reBlackFormat.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Black would reformat: " + strings.TrimSpace(m[1]))
	}
	if m := reTerraformErr.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("Terraform error: " + strings.TrimSpace(m[1]))
	}
	if headline := eslintHeadline(out); headline != "" {
		return trimHeadline(headline)
	}
	if m := reRuffIssue.FindStringSubmatch(out); len(m) == 6 {
		return trimHeadline(fmt.Sprintf("%s:%s:%s: %s %s", strings.TrimSpace(m[1]), m[2], m[3], strings.TrimSpace(m[4]), strings.TrimSpace(m[5])))
	}
	if m := reNpmMissingScript.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline("npm missing script: " + strings.TrimSpace(m[1]))
	}
	if m := reFileLineCol.FindStringSubmatch(out); len(m) == 5 {
		return trimHeadline(fmt.Sprintf("%s:%s: %s", strings.TrimSpace(m[1]), m[2], strings.TrimSpace(m[4])))
	}
	if m := reFileLine.FindStringSubmatch(out); len(m) == 4 {
		return trimHeadline(fmt.Sprintf("%s:%s: %s", strings.TrimSpace(m[1]), m[2], strings.TrimSpace(m[3])))
	}
	if m := reFirstError.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline(strings.TrimSpace(m[1]))
	}
	if m := reJestBullet.FindStringSubmatch(out); len(m) == 2 {
		return trimHeadline(strings.TrimSpace(m[1]))
	}

	return ""
}

func eslintHeadline(out string) string {
	lines := strings.Split(out, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if !reEslintFile.MatchString(line) {
			continue
		}
		for j := i + 1; j < len(lines) && j <= i+6; j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" {
				continue
			}
			if m := reEslintIssue.FindStringSubmatch(next); len(m) == 3 {
				return fmt.Sprintf("%s:%s: %s", line, m[1], strings.TrimSpace(m[2]))
			}
		}
	}
	return ""
}

func trimHeadline(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= headlineMaxLen {
		return s
	}
	return s[:headlineMaxLen-3] + "..."
}
