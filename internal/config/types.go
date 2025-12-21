package config

import "time"

type Config struct {
	Version int     `yaml:"version"`
	Meta    Meta    `yaml:"meta,omitempty"`
	Checks  []Check `yaml:"checks"`
	Runner  Runner  `yaml:"runner,omitempty"`
	Insults Insults `yaml:"insults"`
	Banter  Banter  `yaml:"banter"`
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
	File   string `yaml:"file"`   // .buildbouncer/assets/banter/default.json
	Locale string `yaml:"locale"` // en
}
