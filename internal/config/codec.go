// Package config parses and persists build-bouncer configuration files.
package config

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/storage"
	"gopkg.in/yaml.v3"
)

func Parse(data []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, errors.New("config contains multiple YAML documents; expected exactly one")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}
	if err := validateAndDefault(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	return persist(path, cfg, false)
}

func Create(path string, cfg *Config) error {
	return persist(path, cfg, true)
}

func persist(path string, cfg *Config, createOnly bool) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("config save path is required")
	}
	if cfg == nil {
		return errors.New("config is nil")
	}
	if err := validateAndDefault(cfg); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if createOnly {
		return storage.CreateFile(path, data, 0o600)
	}
	return storage.WriteFile(path, data, 0o600)
}
