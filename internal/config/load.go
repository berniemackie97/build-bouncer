package config

import (
	"errors"
	"fmt"
	"os"
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
	}

	if strings.TrimSpace(cfg.Insults.Mode) == "" {
		cfg.Insults.Mode = "snarky"
	}
	if strings.TrimSpace(cfg.Insults.File) == "" {
		cfg.Insults.File = "assets/insults/default.json"
	}
	if strings.TrimSpace(cfg.Insults.Locale) == "" {
		cfg.Insults.Locale = "en"
	}

	if strings.TrimSpace(cfg.Banter.File) == "" {
		cfg.Banter.File = "assets/banter/default.json"
	}
	if strings.TrimSpace(cfg.Banter.Locale) == "" {
		cfg.Banter.Locale = "en"
	}

	return nil
}
