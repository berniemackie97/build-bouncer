package config

type Config struct {
	Version int     `yaml:"version"`
	Checks  []Check `yaml:"checks"`
	Insults Insults `yaml:"insults"`
	Banter  Banter  `yaml:"banter"`
}

type Check struct {
	Name string            `yaml:"name"`
	Run  string            `yaml:"run"`
	Cwd  string            `yaml:"cwd"`
	Env  map[string]string `yaml:"env"`
}

type Insults struct {
	Mode   string `yaml:"mode"`   // polite | snarky | nuclear
	File   string `yaml:"file"`   // assets/insults/default.json
	Locale string `yaml:"locale"` // en
}

type Banter struct {
	File   string `yaml:"file"`   // assets/banter/default.json
	Locale string `yaml:"locale"` // en
}
