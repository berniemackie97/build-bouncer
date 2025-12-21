package main

import (
	"sort"
	"strconv"
	"strings"

	"build-bouncer/internal/config"
)

type mergeResult struct {
	Merged  []config.Check
	Added   []config.Check
	Skipped []config.Check
}

const manualPlaceholderName = "manual:configure"
const manualPlaceholderSnippet = "TODO: configure build-bouncer checks"

func mergeChecks(existing []config.Check, additions []config.Check) mergeResult {
	seenID := make(map[string]struct{}, len(existing))
	seenContent := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		if id := strings.TrimSpace(c.ID); id != "" {
			seenID[id] = struct{}{}
		}
		seenContent[checkContentKey(c)] = struct{}{}
	}

	merged := append([]config.Check{}, existing...)
	var added []config.Check
	var skipped []config.Check

	for _, c := range additions {
		if id := strings.TrimSpace(c.ID); id != "" {
			if _, ok := seenID[id]; ok {
				skipped = append(skipped, c)
				continue
			}
		}
		key := checkContentKey(c)
		if _, ok := seenContent[key]; ok {
			skipped = append(skipped, c)
			continue
		}
		if id := strings.TrimSpace(c.ID); id != "" {
			seenID[id] = struct{}{}
		}
		seenContent[key] = struct{}{}
		merged = append(merged, c)
		added = append(added, c)
	}

	return mergeResult{Merged: merged, Added: added, Skipped: skipped}
}

func checkContentKey(c config.Check) string {
	run := strings.TrimSpace(c.Run)
	shell := strings.TrimSpace(c.Shell)
	if shell == "" {
		if parsedShell, script, ok := unwrapShellRun(run); ok {
			shell = parsedShell
			run = script
		}
	}
	cwd := strings.TrimSpace(c.Cwd)
	env := normalizeEnvKey(c.Env)
	return shell + "\n" + run + "\n" + cwd + "\n" + env
}

func normalizeEnvKey(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(env[k])
		b.WriteString(";")
	}
	return b.String()
}

func unwrapShellRun(run string) (string, string, bool) {
	if shell, script, ok := parseShellRun("bash", "-lc", run); ok {
		return shell, script, true
	}
	if shell, script, ok := parseShellRun("bash", "-c", run); ok {
		return shell, script, true
	}
	if shell, script, ok := parseShellRun("sh", "-c", run); ok {
		return shell, script, true
	}
	return "", "", false
}

func parseShellRun(shell string, flag string, run string) (string, string, bool) {
	prefix := shell + " " + flag
	if !strings.HasPrefix(run, prefix) {
		return "", "", false
	}
	rest := strings.TrimSpace(run[len(prefix):])
	if rest == "" {
		return "", "", false
	}
	script, ok := unquoteShellArg(rest)
	if !ok {
		return "", "", false
	}
	return shell, script, true
}

func unquoteShellArg(arg string) (string, bool) {
	if len(arg) < 2 || arg[0] != '"' || arg[len(arg)-1] != '"' {
		return "", false
	}
	if s, err := strconv.Unquote(arg); err == nil {
		return s, true
	}
	return arg[1 : len(arg)-1], true
}

func stripManualPlaceholder(checks []config.Check) []config.Check {
	out := make([]config.Check, 0, len(checks))
	for _, c := range checks {
		if isManualPlaceholder(c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func stripCIChecks(checks []config.Check) ([]config.Check, int) {
	out := make([]config.Check, 0, len(checks))
	removed := 0
	for _, c := range checks {
		if isCICheck(c) {
			removed++
			continue
		}
		out = append(out, c)
	}
	return out, removed
}

func isCICheck(c config.Check) bool {
	if strings.HasPrefix(strings.TrimSpace(c.Name), "ci:") {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.Source)), "ci")
}

func isManualPlaceholder(c config.Check) bool {
	if strings.TrimSpace(c.Name) != manualPlaceholderName {
		return false
	}
	return strings.Contains(c.Run, manualPlaceholderSnippet)
}
