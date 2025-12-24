package runner

import "regexp"

// These regexes are intentionally conservative and line anchored where possible.
// Goal is to extract the most helpful hint without false positives taking over.
var (
	// Go test output
	reGoTestFail    = regexp.MustCompile(`(?m)^--- FAIL: ([^\s]+)`)
	reGoTestPkg     = regexp.MustCompile(`(?m)^FAIL\s+([^\s]+)`)
	reGoTestTimeout = regexp.MustCompile(`(?m)^panic: test timed out after ([^\n]+)`)

	// .NET / xUnit-ish output
	reDotnetFail       = regexp.MustCompile(`(?m)^\s*Failed\s+([^\s]+)`)
	reDotnetBuildError = regexp.MustCompile(`(?m)^(.+\.cs)\((\d+),(\d+)\):\s*error\s*CS\d+:\s*(.+)$`)

	// Python / pytest output
	rePytestFail = regexp.MustCompile(`(?m)^FAILED\s+(.+)$`)

	// Jest output
	reJestFail   = regexp.MustCompile(`(?m)^FAIL\s+(.+)$`)
	reJestBullet = regexp.MustCompile(`(?m)^\s+(.+)$`)

	// Generic first headline-ish error line
	reFirstError = regexp.MustCompile(`(?mi)^\s*(?:error|fatal|panic):\s*(.+)$`)

	// TypeScript compiler errors: file(line,col): error TS####: message
	reTscError = regexp.MustCompile(`(?m)^(.+\.tsx?)\((\d+),(\d+)\):\s*error\s*TS\d+:\s*(.+)$`)

	// Rust errors
	reRustError    = regexp.MustCompile(`(?m)^error(?:\[[^\]]+\])?:\s*(.+)$`)
	reRustLocation = regexp.MustCompile(`(?m)^\s*-->\s+(.+):(\d+):(\d+)`)

	// C/C++ compiler errors (gcc/clang style)
	reGccError = regexp.MustCompile(`(?m)^(.+):(\d+):(\d+):\s*error:\s*(.+)$`)

	// General file:line:col and file:line patterns (fallbacks)
	reFileLineCol = regexp.MustCompile(`(?m)^(.+):(\d+):(\d+):\s*(.+)$`)
	reFileLine    = regexp.MustCompile(`(?m)^(.+):(\d+):\s*(.+)$`)

	// Python formatter (black)
	reBlackFormat = regexp.MustCompile(`(?m)^would reformat (.+)$`)

	// Terraform
	reTerraformErr = regexp.MustCompile(`(?m)^Error:\s+(.+)$`)

	// Ruff linter
	reRuffIssue = regexp.MustCompile(`(?m)^(.+):(\d+):(\d+):\s*([A-Z]\d+)\s+(.+)$`)

	// Maven compiler output (common pattern)
	reMavenError = regexp.MustCompile(`(?m)^\[ERROR\]\s+(.+):\[(\d+),(\d+)\]\s+(.+)$`)

	// npm missing script
	reNpmMissingScript = regexp.MustCompile(`(?m)Missing script:\s+"([^"]+)"`)

	// ESLint
	reEslintIssue = regexp.MustCompile(`(?m)^\s*(\d+:\d+)\s+error\s+(.+)$`)
	reEslintFile  = regexp.MustCompile(`\.(?:js|jsx|ts|tsx|mjs|cjs)$`)
)
