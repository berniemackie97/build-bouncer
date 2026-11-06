package main

import (
	"sort"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/shell"
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
	shellSpec := strings.TrimSpace(c.Shell)
	if shellSpec == "" {
		if executable, args, matched, err := shell.ParseDirectInvocation(run); matched && err == nil &&
			len(args) == 2 && (executable == "bash" || executable == "sh") {
			shellSpec = executable
			run = args[1]
		}
	}
	cwd := strings.TrimSpace(c.Cwd)
	env := normalizeEnvKey(c.Env)
	key := shellSpec + "\n" + run + "\n" + cwd + "\n" + env + "\n" + normalizeStringListKey(c.OS)
	if c.AllowMissingTools {
		key += "\nallowMissingTools"
	}
	return key
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
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.Source)), "ci:")
}

func isManualPlaceholder(c config.Check) bool {
	if strings.TrimSpace(c.Name) != manualPlaceholderName {
		return false
	}
	return strings.Contains(c.Run, manualPlaceholderSnippet)
}
