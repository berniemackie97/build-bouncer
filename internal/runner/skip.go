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
	reason, _ := checkReadiness(check)
	return reason
}

func checkReadiness(check config.Check) (reason string, unavailable bool) {
	applies, allowedOSValues := checkAppliesToOS(check)
	if !applies {
		if len(allowedOSValues) == 0 {
			return "os mismatch", false
		}
		return "os mismatch (want " + strings.Join(allowedOSValues, ",") + ")", false
	}

	if _, err := ResolveCheckExecutable(check.Shell, check.Run); err != nil {
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
	rawOSValues := make([]string, 0, len(check.OS)+len(check.Platforms))
	rawOSValues = append(rawOSValues, check.OS...)
	rawOSValues = append(rawOSValues, check.Platforms...)
	if len(rawOSValues) == 0 {
		return true, nil
	}

	allowed := normalizeAllowedOSValues(rawOSValues)
	if len(allowed) == 0 {
		// If config values are present but all are junk, treat as "no constraint".
		return true, nil
	}

	current := currentOS()
	if slices.Contains(allowed, current) {
		return true, allowed
	}
	return false, allowed
}

func normalizeAllowedOSValues(values []string) []string {
	allowed := make([]string, 0, len(values))
	seen := map[string]struct{}{}

	for _, rawValue := range values {
		normalizedValue, ok := normalizeOSValue(rawValue)
		if !ok {
			continue
		}
		if _, exists := seen[normalizedValue]; exists {
			continue
		}
		seen[normalizedValue] = struct{}{}
		allowed = append(allowed, normalizedValue)
	}

	return allowed
}

func normalizeOSValue(value string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return "", false
	}

	switch {
	case strings.Contains(lower, "windows"):
		return "windows", true
	case strings.Contains(lower, "macos") || strings.Contains(lower, "osx") || strings.Contains(lower, "darwin"):
		return "macos", true
	case strings.Contains(lower, "linux") || strings.Contains(lower, "ubuntu"):
		return "linux", true
	default:
		return "", false
	}
}

func currentOS() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "macos"
	default:
		return "linux"
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
