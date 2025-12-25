package config

import "time"

type Config struct {
	Version    int        `yaml:"version"`
	Meta       Meta       `yaml:"meta,omitempty"`
	Checks     []Check    `yaml:"checks"`
	Runner     Runner     `yaml:"runner,omitempty"`
	Protection Protection `yaml:"protection,omitempty"`
	Insults    Insults    `yaml:"insults"`
	Banter     Banter     `yaml:"banter"`
}

type Check struct {
	ID        string            `yaml:"id,omitempty"`
	Source    string            `yaml:"source,omitempty"`
	Name      string            `yaml:"name"`
	Run       string            `yaml:"run"`
	Shell     string            `yaml:"shell,omitempty"`
	Cwd       string            `yaml:"cwd,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	OS        StringList        `yaml:"os,omitempty"`
	Platforms StringList        `yaml:"platforms,omitempty"`
	Requires  StringList        `yaml:"requires,omitempty"`
	Timeout   time.Duration     `yaml:"timeout,omitempty"`
}

type Runner struct {
	MaxParallel int  `yaml:"maxParallel,omitempty"`
	FailFast    bool `yaml:"failFast,omitempty"`
}

type Meta struct {
	Template TemplateMeta      `yaml:"template,omitempty"`
	Inputs   map[string]string `yaml:"inputs,omitempty"`
}

type TemplateMeta struct {
	ID string `yaml:"id,omitempty"`
}

type Insults struct {
	Mode   string `yaml:"mode"`   // polite | snarky | nuclear
	File   string `yaml:"file"`   // .buildbouncer/assets/insults/default.json
	Locale string `yaml:"locale"` // en
}

type Banter struct {
	Enabled *bool  `yaml:"enabled"`
	File    string `yaml:"file"`   // .buildbouncer/assets/banter/default.json
	Locale  string `yaml:"locale"` // en
}

// Protection defines the enforcement level for build-bouncer checks.
//
// Levels (from least to most strict):
//   - lax: Only blocks on build/compilation errors (critical failures)
//   - moderate: Blocks on build errors, test failures, and CI checks (default)
//   - strict: Blocks on any check failure, no override prompts in hook mode
//
// Interactive mode (manual `build-bouncer check` command):
//   - lax/moderate: Shows interactive prompt allowing override on failures
//   - strict: No interactive override, must use --force-push flag
//
// Hook mode (git pre-push):
//   - lax: Only blocks on critical build failures
//   - moderate: Blocks on tests/CI, allows override via BUILDBOUNCER_SKIP=1
//   - strict: Blocks on any failure, no prompts, requires --force-push flag
type Protection struct {
	// Level determines how strictly checks are enforced: lax, moderate, strict
	Level string `yaml:"level,omitempty"`

	// Interactive enables override prompts in non-hook mode (default: true for lax/moderate, false for strict)
	Interactive *bool `yaml:"interactive,omitempty"`

	// CriticalPatterns are regex patterns to identify critical failures (build errors, compilation failures)
	// Used by 'lax' mode to determine if a failure is severe enough to block
	CriticalPatterns []string `yaml:"criticalPatterns,omitempty"`
}

// ProtectionLevel returns the configured protection level, defaulting to "moderate"
func (p Protection) ProtectionLevel() string {
	if p.Level == "" {
		return "moderate"
	}
	return p.Level
}

// IsInteractive returns whether interactive prompts should be shown
func (p Protection) IsInteractive() bool {
	if p.Interactive != nil {
		return *p.Interactive
	}
	// Default: interactive for lax/moderate, non-interactive for strict
	return p.ProtectionLevel() != "strict"
}
