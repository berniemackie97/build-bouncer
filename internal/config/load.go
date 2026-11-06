package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/shell"
	"unicode"
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

		// Config is user-authored: keep shell strict and predictable.
		// shell must be ONLY the executable name/path (no args).
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
		cwd, err := validateCheckDirectory(c.Cwd)
		if err != nil {
			return fmt.Errorf("config: checks[%d] cwd: %w", i, err)
		}

		// Normalize deprecated/legacy fields into canonical ones.
		c.Name = name
		c.Cwd = cwd
		c.OS = osList
		c.Platforms = nil
		c.Requires = requires

		cfg.Checks[i] = c
	}

	if cfg.Runner.MaxParallel < 0 {
		return errors.New("config: runner.maxParallel must be >= 0")
	}
	if cfg.Runner.MaxLogBytes < 0 || cfg.Runner.MaxSavedLogs < 0 || cfg.Runner.LogRetentionDays < 0 {
		return errors.New("config: runner log limits must be >= 0")
	}

	cfg.Protection.Level = strings.ToLower(strings.TrimSpace(cfg.Protection.Level))
	switch cfg.Protection.Level {
	case "", "lax", "moderate", "strict":
	default:
		return fmt.Errorf("config: protection.level %q is invalid (expected lax, moderate, or strict)", cfg.Protection.Level)
	}

	cfg.Insults.Mode = strings.ToLower(strings.TrimSpace(cfg.Insults.Mode))
	if cfg.Insults.Mode == "" {
		cfg.Insults.Mode = "snarky"
	}
	switch cfg.Insults.Mode {
	case "polite", "snarky", "nuclear":
	default:
		return fmt.Errorf("config: insults.mode %q is invalid (expected polite, snarky, or nuclear)", cfg.Insults.Mode)
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

func validateShellSpec(spec string) error {
	_, err := shell.ParseExecutable(spec)
	return err
}

func validateEnvOverrides(env map[string]string) error {
	for key, value := range env {
		if key == "" || strings.IndexFunc(key, unicode.IsSpace) >= 0 || strings.ContainsAny(key, "\x00=") {
			return fmt.Errorf("env key %q contains invalid characters", key)
		}
		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("env value for %q contains NUL", key)
		}
	}
	return nil
}

func validateCheckDirectory(cwd string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", nil
	}
	if strings.ContainsAny(cwd, "\x00\r\n\t") {
		return "", errors.New("must not contain control characters")
	}
	normalized := strings.ReplaceAll(cwd, "\\", "/")
	if path.IsAbs(normalized) || len(normalized) >= 2 && normalized[1] == ':' {
		return "", errors.New("must be relative to the repository root")
	}
	normalized = path.Clean(normalized)
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", errors.New("must not escape the repository root")
	}
	return normalized, nil
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
