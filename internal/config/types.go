package config

import "time"

type Config struct {
	Version int     `yaml:"version"`
	Checks  []Check `yaml:"checks"`
	Runner  Runner  `yaml:"runner,omitempty"`
	Insults Insults `yaml:"insults"`
	Banter  Banter  `yaml:"banter"`
}

type Check struct {
	Name    string            `yaml:"name"`
	Run     string            `yaml:"run"`
	Shell   string            `yaml:"shell,omitempty"`
	Cwd     string            `yaml:"cwd,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Timeout time.Duration     `yaml:"timeout,omitempty"`
}

type Runner struct {
	MaxParallel int  `yaml:"maxParallel,omitempty"`
	FailFast    bool `yaml:"failFast,omitempty"`
}

type Insults struct {
	Mode   string `yaml:"mode"`   // polite | snarky | nuclear
	File   string `yaml:"file"`   // .buildbouncer/assets/insults/default.json
	Locale string `yaml:"locale"` // en
}

type Banter struct {
	File   string `yaml:"file"`   // .buildbouncer/assets/banter/default.json
	Locale string `yaml:"locale"` // en
}
