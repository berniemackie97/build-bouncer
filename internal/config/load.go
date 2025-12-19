package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validate(cfg *Config) error {
	if cfg.Version <= 0 {
		return errors.New("config: missing/invalid version")
	}

	if len(cfg.Checks) == 0 {
		return errors.New("config: no checks configured")
	}

	for i, c := range cfg.Checks {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("config: checks[%d] missing name", i)
		}
		if strings.TrimSpace(c.Run) == "" {
			return fmt.Errorf("config: checks[%d] missing run", i)
		}
	}

	if strings.TrimSpace(cfg.Insults.Mode) == "" {
		cfg.Insults.Mode = "snarky"
	}
	if strings.TrimSpace(cfg.Insults.File) == "" {
		cfg.Insults.File = "assets/insults/default.txt"
	}

	return nil
}
