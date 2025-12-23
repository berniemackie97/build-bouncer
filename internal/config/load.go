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

	for i := range cfg.Checks {
		c := cfg.Checks[i]
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("config: checks[%d] missing name", i)
		}
		if strings.TrimSpace(c.Run) == "" {
			return fmt.Errorf("config: checks[%d] missing run", i)
		}
		if err := validateShell(c.Shell); err != nil {
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

func validateShell(shell string) error {
	s := strings.TrimSpace(shell)
	if s == "" {
		return nil
	}
	if strings.ContainsAny(s, "\r\n\t") {
		return errors.New("must be a single executable name or path")
	}
	if strings.ContainsAny(s, " ") && !strings.ContainsAny(s, `/\`) {
		return errors.New("must not include arguments (use only the shell executable)")
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
