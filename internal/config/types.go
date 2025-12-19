package config

type Config struct {
	Version int      `yaml:"version"`
	Checks  []Check  `yaml:"checks"`
	Insults []Insult `yaml:"insults"`
}

type Check struct {
	Name string            `yaml:"name"`
	Run  string            `yaml:"run"`
	Cwd  string            `yaml:"cwd"`
	Env  map[string]string `yaml:"env"`
}

type Insults struct {
	Mode string `yaml:"mode"`
	File string `yaml:"file"`
}
