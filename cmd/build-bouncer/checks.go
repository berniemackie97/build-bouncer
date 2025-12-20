package main

import (
	"sort"
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
	seen := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		seen[checkKey(c)] = struct{}{}
	}

	merged := append([]config.Check{}, existing...)
	var added []config.Check
	var skipped []config.Check

	for _, c := range additions {
		key := checkKey(c)
		if _, ok := seen[key]; ok {
			skipped = append(skipped, c)
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, c)
		added = append(added, c)
	}

	return mergeResult{Merged: merged, Added: added, Skipped: skipped}
}

func checkKey(c config.Check) string {
	run := strings.TrimSpace(c.Run)
	cwd := strings.TrimSpace(c.Cwd)
	env := normalizeEnvKey(c.Env)
	return run + "\n" + cwd + "\n" + env
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

func isManualPlaceholder(c config.Check) bool {
	if strings.TrimSpace(c.Name) != manualPlaceholderName {
		return false
	}
	return strings.Contains(c.Run, manualPlaceholderSnippet)
}
