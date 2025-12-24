// Package config parses and persists build-bouncer configuration files.
package config

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func Parse(data []byte) (*Config, error) {
	var cfg Config

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // reject unknown fields (typos should fail loudly)

	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}

	// Reject multiple YAML documents (--- ... --- ...), which almost always indicates
	// a mistaken concat or copy/paste.
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
	if strings.TrimSpace(path) == "" {
		return errors.New("config save path is required")
	}
	if cfg == nil {
		return errors.New("config is nil")
	}

	if err := validateAndDefault(cfg); err != nil {
		return err
	}

	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// Friendly POSIX convention (also helps diffs).
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		// Preserve existing permissions (but don't preserve weird mode bits).
		mode = st.Mode().Perm()
	}

	return writeFileAtomic(path, b, mode)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmpPath := path + ".tmp"
	backupPath := path + ".bak"

	_ = os.Remove(tmpPath)

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	_, writeErr := out.Write(data)
	syncErr := out.Sync()
	closeErr := out.Close()

	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return writeErr
	}
	if syncErr != nil {
		_ = os.Remove(tmpPath)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	// Windows: rename-over-existing is not allowed. Move old aside first.
	_ = os.Remove(backupPath)
	if err := os.Rename(path, backupPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		// Best-effort restore
		_ = os.Rename(backupPath, path)
		return err
	}

	_ = os.Remove(backupPath)
	_ = os.Chmod(path, mode)
	return nil
}
