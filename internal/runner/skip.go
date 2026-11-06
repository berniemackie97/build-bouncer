package runner

import (
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/shell"
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

	missing, err := missingToolsAt(repoRoot, check)
	if err != nil {
		return "invalid required executable: " + err.Error(), true
	}
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
	return MissingToolsAt("", check)
}

func MissingToolsAt(repoRoot string, check config.Check) []string {
	missing, err := missingToolsAt(repoRoot, check)
	if err != nil {
		return []string{err.Error()}
	}
	return missing
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

func missingToolsAt(repoRoot string, check config.Check) ([]string, error) {
	seen := make(map[string]struct{}, len(check.Requires))
	var missing []string

	for _, required := range check.Requires {
		executable, err := shell.ParseExecutable(required)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", required, err)
		}
		if executable == "" {
			return nil, fmt.Errorf("empty required executable")
		}
		resolved := resolveExecutableAt(repoRoot, executable)
		if _, exists := seen[resolved]; exists {
			continue
		}
		seen[resolved] = struct{}{}
		if _, err := exec.LookPath(resolved); err != nil {
			missing = append(missing, executable)
		}
	}

	return missing, nil
}
