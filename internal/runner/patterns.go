package runner

import "regexp"

var (
	reGoTestFail   = regexp.MustCompile(`(?m)^--- FAIL: ([^\s]+)`)
	reGoTestPkg    = regexp.MustCompile(`(?m)^FAIL\s+([^\s]+)`)
	reDotnetFail   = regexp.MustCompile(`(?m)^\s*Failed\s+([^\s]+)`)
	rePytestFail   = regexp.MustCompile(`(?m)^FAILED\s+(.+)$`)
	reJestFail     = regexp.MustCompile(`(?m)^FAIL\s+(.+)$`)
	reJestBullet   = regexp.MustCompile(`(?m)^●\s+(.+)$`)
	reFirstError   = regexp.MustCompile(`(?mi)^\s*(?:error|fatal|panic):\s*(.+)$`)
	reTscError     = regexp.MustCompile(`(?m)^(.+\.tsx?)\((\d+),(\d+)\):\s*error\s*TS\d+:\s*(.+)$`)
	reRustError    = regexp.MustCompile(`(?m)^error(?:\[[^\]]+\])?:\s*(.+)$`)
	reGccError     = regexp.MustCompile(`(?m)^(.+):(\d+):(\d+):\s*error:\s*(.+)$`)
	reFileLineCol  = regexp.MustCompile(`(?m)^(.+):(\d+):(\d+):\s*(.+)$`)
	reFileLine     = regexp.MustCompile(`(?m)^(.+):(\d+):\s*(.+)$`)
	reBlackFormat  = regexp.MustCompile(`(?m)^would reformat (.+)$`)
	reTerraformErr = regexp.MustCompile(`(?m)^Error:\s+(.+)$`)
	reEslintIssue  = regexp.MustCompile(`^\s*(\d+:\d+)\s+error\s+(.+)$`)
	reEslintFile   = regexp.MustCompile(`\.(?:js|jsx|ts|tsx|mjs|cjs)$`)
)
