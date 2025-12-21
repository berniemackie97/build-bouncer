package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"build-bouncer/internal/config"
)

func checkSkipReason(check config.Check) string {
	if ok, allowed := checkAppliesToOS(check); !ok {
		if len(allowed) == 0 {
			return "os mismatch"
		}
		return "os mismatch (want " + strings.Join(allowed, ",") + ")"
	}
	if missing := missingTools(check); len(missing) > 0 {
		return "missing tools: " + strings.Join(missing, ", ")
	}
	return ""
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
	values := make([]string, 0, len(check.OS)+len(check.Platforms))
	values = append(values, check.OS...)
	values = append(values, check.Platforms...)
	if len(values) == 0 {
		return true, nil
	}

	allowed := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, item := range values {
		val, ok := normalizeOSValue(item)
		if !ok {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		allowed = append(allowed, val)
	}
	if len(allowed) == 0 {
		return true, nil
	}
	current := currentOS()
	for _, v := range allowed {
		if v == current {
			return true, allowed
		}
	}
	return false, allowed
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
	seen := map[string]struct{}{}
	var missing []string

	for _, tool := range check.Requires {
		t := firstToken(tool)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		if !toolExists(t) {
			missing = append(missing, t)
		}
	}

	if shell := strings.TrimSpace(check.Shell); shell != "" {
		base := strings.ToLower(filepath.Base(shell))
		if base != "cmd" && base != "cmd.exe" {
			if t := firstToken(shell); t != "" {
				if _, ok := seen[t]; !ok {
					seen[t] = struct{}{}
					if !toolExists(t) {
						missing = append(missing, t)
					}
				}
			}
		}
	}

	return missing
}

func toolExists(tool string) bool {
	if strings.ContainsAny(tool, `/\`) {
		_, err := os.Stat(tool)
		return err == nil
	}
	_, err := exec.LookPath(tool)
	return err == nil
}

func firstToken(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
