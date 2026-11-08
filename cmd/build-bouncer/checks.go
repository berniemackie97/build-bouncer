package main

import (
	"fmt"
	"sort"
	"strconv"
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

func mergeChecks(existing []config.Check, additions []config.Check) (mergeResult, error) {
	seenID := make(map[string]string, len(existing))
	seenContent := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		key := checkContentKey(c)
		if id := strings.TrimSpace(c.ID); id != "" {
			if previous, ok := seenID[id]; ok && previous != key {
				return mergeResult{}, fmt.Errorf("check ID %q has conflicting content", id)
			}
			seenID[id] = key
		}
		seenContent[key] = struct{}{}
	}

	merged := append([]config.Check{}, existing...)
	var added []config.Check
	var skipped []config.Check

	for _, c := range additions {
		key := checkContentKey(c)
		if id := strings.TrimSpace(c.ID); id != "" {
			if previous, ok := seenID[id]; ok {
				if previous != key {
					return mergeResult{}, fmt.Errorf("check ID %q has conflicting content", id)
				}
				skipped = append(skipped, c)
				continue
			}
		}
		if _, ok := seenContent[key]; ok {
			skipped = append(skipped, c)
			continue
		}
		if id := strings.TrimSpace(c.ID); id != "" {
			seenID[id] = key
		}
		seenContent[key] = struct{}{}
		merged = append(merged, c)
		added = append(added, c)
	}

	return mergeResult{Merged: merged, Added: added, Skipped: skipped}, nil
}

func checkContentKey(c config.Check) string {
	run := strings.TrimSpace(c.Run)
	shellSpec := strings.TrimSpace(c.Shell)
	if shellSpec == "" {
		if executable, args, matched, err := shell.ParseDirectInvocation(run); matched && err == nil && len(args) == 2 {
			if (executable == "bash" && args[0] == "-lc") || (executable == "sh" && args[0] == "-c") {
				shellSpec = executable
				run = args[1]
			}
		}
	}

	var key strings.Builder
	for _, part := range []string{shellSpec, run, strings.TrimSpace(c.Cwd)} {
		appendContentKeyPart(&key, part)
	}
	envKeys := make([]string, 0, len(c.Env))
	for name := range c.Env {
		envKeys = append(envKeys, name)
	}
	sort.Strings(envKeys)
	appendContentKeyPart(&key, strconv.Itoa(len(envKeys)))
	for _, name := range envKeys {
		appendContentKeyPart(&key, name)
		appendContentKeyPart(&key, c.Env[name])
	}
	for _, values := range []config.StringList{c.OS, c.Requires} {
		normalized := make([]string, 0, len(values))
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				normalized = append(normalized, value)
			}
		}
		sort.Strings(normalized)
		appendContentKeyPart(&key, strconv.Itoa(len(normalized)))
		for _, value := range normalized {
			appendContentKeyPart(&key, value)
		}
	}
	appendContentKeyPart(&key, strconv.FormatBool(c.AllowMissingTools))
	appendContentKeyPart(&key, strconv.FormatBool(c.AllowFailure))
	appendContentKeyPart(&key, strconv.FormatInt(int64(c.Timeout), 10))
	return key.String()
}

func appendContentKeyPart(key *strings.Builder, value string) {
	key.WriteString(strconv.Itoa(len(value)))
	key.WriteByte(':')
	key.WriteString(value)
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

func stripCIChecks(checks []config.Check) ([]config.Check, int, error) {
	out := make([]config.Check, 0, len(checks))
	removed := 0
	for _, c := range checks {
		if isCICheck(c) {
			if c.ID != "" && c.ID != stableCheckID(c.Source, c) {
				return nil, 0, fmt.Errorf("CI-derived check %q has been modified; preserve it as a custom check before syncing", c.Name)
			}
			removed++
			continue
		}
		out = append(out, c)
	}
	return out, removed, nil
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
