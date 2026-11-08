package config

import "time"

const (
	MaxParallelChecks   = 32
	MaxCheckTimeout     = 2 * time.Hour
	MaxLogBytes   int64 = 64 << 20
	MaxSavedLogs        = 100
	MaxLogRetentionDays = 365
)

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
	ID                string            `yaml:"id,omitempty"`
	Source            string            `yaml:"source,omitempty"`
	Name              string            `yaml:"name"`
	Run               string            `yaml:"run"`
	Shell             string            `yaml:"shell,omitempty"`
	Cwd               string            `yaml:"cwd,omitempty"`
	Env               map[string]string `yaml:"env,omitempty"`
	OS                StringList        `yaml:"os,omitempty"`
	Platforms         StringList        `yaml:"platforms,omitempty"`
	Requires          StringList        `yaml:"requires,omitempty"`
	AllowMissingTools bool              `yaml:"allowMissingTools,omitempty"`
	AllowFailure      bool              `yaml:"allowFailure,omitempty"`
	Timeout           time.Duration     `yaml:"timeout,omitempty"`
}

type Runner struct {
	MaxParallel      int   `yaml:"maxParallel,omitempty"`
	FailFast         bool  `yaml:"failFast,omitempty"`
	MaxLogBytes      int64 `yaml:"maxLogBytes,omitempty"`
	MaxSavedLogs     int   `yaml:"maxSavedLogs,omitempty"`
	LogRetentionDays int   `yaml:"logRetentionDays,omitempty"`
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

// Protection controls failure enforcement and interactive hook overrides.
type Protection struct {
	Level string `yaml:"level,omitempty"`

	Interactive *bool `yaml:"interactive,omitempty"`

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
	if p.ProtectionLevel() == "strict" {
		return false
	}
	if p.Interactive != nil {
		return *p.Interactive
	}
	return p.ProtectionLevel() != "strict"
}
