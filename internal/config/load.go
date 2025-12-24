package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// validateAndDefault mutates cfg in-place (defaults + normalization).
func validateAndDefault(cfg *Config) error {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Version != 1 {
		return fmt.Errorf("config: unsupported version %d", cfg.Version)
	}

	if len(cfg.Checks) == 0 {
		return errors.New("config: no checks configured")
	}

	// Enforce unique check names (maps in Report key on check name).
	seenNames := make(map[string]int, len(cfg.Checks))

	for i := range cfg.Checks {
		c := cfg.Checks[i]

		name := strings.TrimSpace(c.Name)
		if name == "" {
			return fmt.Errorf("config: checks[%d] missing name", i)
		}
		if prev, exists := seenNames[name]; exists {
			return fmt.Errorf("config: checks[%d] name %q duplicates checks[%d] (check names must be unique)", i, name, prev)
		}
		seenNames[name] = i

		if strings.TrimSpace(c.Run) == "" {
			return fmt.Errorf("config: checks[%d] missing run", i)
		}

		// Note: runner supports shellSpec with optional prefix args (e.g. `cmd /D`,
		// `"C:\Program Files\PowerShell\7\pwsh.exe" -NoProfile`), so validation must allow that,
		// while still rejecting obviously broken / unsafe input (newlines, NUL, unterminated quotes, etc).
		if err := validateShellSpec(c.Shell); err != nil {
			return fmt.Errorf("config: checks[%d] shell: %w", i, err)
		}

		osList, err := normalizeOSList(c.OS, c.Platforms)
		if err != nil {
			return fmt.Errorf("config: checks[%d] os: %w", i, err)
		}

		requires, err := normalizeStringList(c.Requires)
		if err != nil {
			return fmt.Errorf("config: checks[%d] requires: %w", i, err)
		}

		if c.Timeout < 0 {
			return fmt.Errorf("config: checks[%d] timeout must be >= 0", i)
		}

		if err := validateEnvOverrides(c.Env); err != nil {
			return fmt.Errorf("config: checks[%d] env: %w", i, err)
		}

		// Normalize deprecated/legacy fields into canonical ones.
		c.Name = name
		c.OS = osList
		c.Platforms = nil
		c.Requires = requires

		cfg.Checks[i] = c
	}

	if cfg.Runner.MaxParallel < 0 {
		return errors.New("config: runner.maxParallel must be >= 0")
	}

	if strings.TrimSpace(cfg.Insults.Mode) == "" {
		cfg.Insults.Mode = "snarky"
	}
	if strings.TrimSpace(cfg.Insults.File) == "" {
		cfg.Insults.File = filepath.ToSlash(filepath.Join(ConfigDirName, DefaultInsultsRel))
	}
	if strings.TrimSpace(cfg.Insults.Locale) == "" {
		cfg.Insults.Locale = "en"
	}

	if strings.TrimSpace(cfg.Banter.File) == "" {
		cfg.Banter.File = filepath.ToSlash(filepath.Join(ConfigDirName, DefaultBanterRel))
	}
	if strings.TrimSpace(cfg.Banter.Locale) == "" {
		cfg.Banter.Locale = "en"
	}
	if cfg.Banter.Enabled == nil {
		enabled := true
		cfg.Banter.Enabled = &enabled
	}

	return nil
}

func validateShellSpec(shellSpec string) error {
	s := strings.TrimSpace(shellSpec)
	if s == "" {
		return nil
	}

	// Config is user-authored: be strict here. Runner code can be more tolerant at runtime,
	// but config should fail fast on clearly broken input.
	if strings.ContainsAny(s, "\x00\r\n\t") {
		return errors.New("must be a single line (no NUL/newlines/tabs)")
	}

	exe, _, err := cutFirstTokenStrict(s)
	if err != nil {
		return err
	}
	if strings.TrimSpace(exe) == "" {
		return errors.New("must include an executable name or path")
	}
	if strings.HasPrefix(exe, "-") {
		return errors.New("executable must not start with '-'")
	}

	return nil
}

// cutFirstTokenStrict returns the first token and the remaining text.
// It supports quoting for the first token, but requires quotes to be terminated.
func cutFirstTokenStrict(text string) (string, string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", "", nil
	}

	first := trimmed[0]
	if first == '"' || first == '\'' {
		quote := first
		rest := trimmed[1:]
		closing := strings.IndexByte(rest, quote)
		if closing == -1 {
			return "", "", errors.New("unterminated quoted executable path")
		}
		token := rest[:closing]
		remaining := strings.TrimSpace(trimmed[2+closing:])
		return token, remaining, nil
	}

	// Unquoted: token is up to first whitespace.
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] == ' ' {
			return trimmed[:i], strings.TrimSpace(trimmed[i+1:]), nil
		}
	}
	return trimmed, "", nil
}

func validateEnvOverrides(env map[string]string) error {
	if len(env) == 0 {
		return nil
	}

	for rawKey := range env {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return errors.New("env contains an empty key")
		}
		if strings.ContainsAny(key, "\x00\r\n") {
			return fmt.Errorf("env key %q contains invalid characters", rawKey)
		}
		if strings.Contains(key, "=") {
			return fmt.Errorf("env key %q must not contain '='", rawKey)
		}
	}

	return nil
}

func normalizeStringList(list StringList) (StringList, error) {
	if len(list) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(list))
	seen := map[string]struct{}{}
	for _, item := range list {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func normalizeOSList(osList StringList, platforms StringList) (StringList, error) {
	combined := make([]string, 0, len(osList)+len(platforms))
	combined = append(combined, osList...)
	combined = append(combined, platforms...)
	if len(combined) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(combined))
	seen := map[string]struct{}{}
	for _, item := range combined {
		val, ok := normalizeOSValue(item)
		if !ok {
			return nil, fmt.Errorf("unknown value %q", item)
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out, nil
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
