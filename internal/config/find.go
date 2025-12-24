// Package config contains configuration discovery, parsing, validation, and persistence helpers.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrConfigNotFound = errors.New("build-bouncer config not found")

func FindConfigFromCwd() (cfgPath string, cfgDir string, err error) {
	start, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	// Best-effort normalize so error messages are stable.
	if abs, absErr := filepath.Abs(start); absErr == nil {
		start = abs
	}
	if real, realErr := filepath.EvalSymlinks(start); realErr == nil {
		start = real
	}

	dir := start
	for {
		candidate := filepath.Join(dir, ConfigDirName, ConfigFileName)
		if st, statErr := os.Stat(candidate); statErr == nil && !st.IsDir() {
			return candidate, dir, nil
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return "", "", fmt.Errorf("stat %q: %w", candidate, statErr)
		}

		legacy := filepath.Join(dir, LegacyConfigName)
		if st, statErr := os.Stat(legacy); statErr == nil && !st.IsDir() {
			return legacy, dir, nil
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return "", "", fmt.Errorf("stat %q: %w", legacy, statErr)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", "", fmt.Errorf(
		"%w: looked from %q up to filesystem root for %q or %q",
		ErrConfigNotFound,
		start,
		filepath.Join(ConfigDirName, ConfigFileName),
		LegacyConfigName,
	)
}
