package config

type Config struct {
	Version int     `yaml:"version"`
	Checks  []Check `yaml:"checks"`
	Insults Insults `yaml:"insults"`
}

type Check struct {
	Name string            `yaml:"name"`
	Run  string            `yaml:"run"`
	Cwd  string            `yaml:"cwd"`
	Env  map[string]string `yaml:"env"`
}

type Insults struct {
	Mode   string `yaml:"mode"`   // polite | snarky | nuclear
	File   string `yaml:"file"`   // path to JSON pack in the target repo
	Locale string `yaml:"locale"` // e.g. "en"
}
