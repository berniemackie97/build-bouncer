package main

import (
	"crypto/sha1"
	"encoding/hex"
	"path"
	"runtime"
	"sort"
	"strings"

	"build-bouncer/internal/config"
	"build-bouncer/internal/shell"
)

func stampGeneratedChecks(checks []config.Check, source string) []config.Check {
	out := make([]config.Check, len(checks))
	for i, check := range checks {
		if strings.TrimSpace(check.Source) == "" && strings.TrimSpace(source) != "" {
			check.Source = source
		}
		check.Shell = resolveGeneratedShell(check)
		if len(check.Requires) == 0 {
			check.Requires = inferRequires(check)
		}
		if strings.TrimSpace(check.ID) == "" {
			idSource := strings.TrimSpace(check.Source)
			if idSource == "" {
				idSource = source
			}
			check.ID = stableCheckID(idSource, check)
		}
		out[i] = check
	}
	return out
}

func resolveGeneratedShell(check config.Check) string {
	if strings.TrimSpace(check.Shell) != "" {
		return shell.Normalize(check.Shell)
	}
	return shell.Resolve(runtime.GOOS, "", check.Run)
}

func stableCheckID(source string, check config.Check) string {
	h := sha1.New()
	parts := []string{
		strings.TrimSpace(source),
		strings.TrimSpace(check.Name),
		strings.TrimSpace(check.Run),
		strings.TrimSpace(check.Shell),
		strings.TrimSpace(check.Cwd),
		normalizeEnvKey(check.Env),
		normalizeStringListKey(check.OS),
		normalizeStringListKey(check.Requires),
	}
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return strings.TrimSpace(source) + ":" + hex.EncodeToString(sum[:6])
}

func normalizeStringListKey(list config.StringList) string {
	if len(list) == 0 {
		return ""
	}
	items := make([]string, 0, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		items = append(items, strings.ToLower(item))
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func inferRequires(check config.Check) config.StringList {
	token := primaryCommand(check.Run)
	if token == "" {
		return nil
	}
	if isShellBuiltin(token) {
		return nil
	}
	if strings.Contains(token, "/") || strings.Contains(token, "\\") || strings.HasPrefix(token, ".") {
		return nil
	}
	return config.StringList{token}
}

func primaryCommand(run string) string {
	fields := strings.Fields(run)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(path.Base(strings.Trim(fields[0], `"'`)))
}

func isShellBuiltin(token string) bool {
	switch token {
	case "cd", "set", "echo", "dir", "if", "for", "call", "exit":
		return true
	case "pwd", "export", "test", "true", "false":
		return true
	default:
		return false
	}
}

func mergeInputs(base map[string]string, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}
