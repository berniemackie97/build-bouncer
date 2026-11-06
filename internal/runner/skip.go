package runner

import (
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func checkSkipReason(check config.Check) string {
	reason, _ := checkReadiness("", check)
	return reason
}

func checkReadiness(repoRoot string, check config.Check) (reason string, unavailable bool) {
	applies, allowedOSValues := checkAppliesToOS(check)
	if !applies {
		if len(allowedOSValues) == 0 {
			return "invalid OS restriction", true
		}
		return "os mismatch (want " + strings.Join(allowedOSValues, ",") + ")", false
	}

	if _, err := ResolveCheckExecutableAt(repoRoot, check.Shell, check.Run, check.Env); err != nil {
		return "shell unavailable: " + err.Error(), true
	}

	missing := missingTools(check)
	if len(missing) > 0 {
		return "missing tools: " + strings.Join(missing, ", "), !check.AllowMissingTools
	}
	return "", false
}

func SkipReason(check config.Check) string {
	return checkSkipReason(check)
}

func SkipReasonAt(repoRoot string, check config.Check) string {
	reason, _ := checkReadiness(repoRoot, check)
	return reason
}

func CheckAppliesToOS(check config.Check) (bool, []string) {
	return checkAppliesToOS(check)
}

func MissingTools(check config.Check) []string {
	return missingTools(check)
}

func CurrentOS() string {
	return currentOS()
}

func checkAppliesToOS(check config.Check) (bool, []string) {
	allowed, err := config.NormalizeOSList(check.OS, check.Platforms)
	if err != nil {
		return false, nil
	}
	if len(allowed) == 0 {
		return true, nil
	}
	return slices.Contains(allowed, currentOS()), allowed
}

func currentOS() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "macos"
	default:
		return runtime.GOOS
	}
}

func missingTools(check config.Check) []string {
	seenTools := map[string]struct{}{}
	missing := make([]string, 0)

	for _, toolSpec := range check.Requires {
		toolName := firstToken(toolSpec)
		if toolName == "" {
			continue
		}
		if _, alreadyChecked := seenTools[toolName]; alreadyChecked {
			continue
		}

		seenTools[toolName] = struct{}{}
		if !toolExists(toolName) {
			missing = append(missing, toolName)
		}
	}

	return missing
}

func toolExists(tool string) bool {
	// If the user gave us a path, check the filesystem.
	if strings.ContainsAny(tool, `/\`) {
		info, err := os.Stat(tool)
		return err == nil && info.Mode().IsRegular()
	}

	_, lookErr := exec.LookPath(tool)
	return lookErr == nil
}

func firstToken(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
