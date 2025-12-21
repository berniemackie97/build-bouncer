package config

import (
	"errors"
	"os"
	"path/filepath"
)

func FindConfigFromCwd() (cfgPath string, cfgDir string, err error) {
	start, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	dir := start
	for {
		candidate := filepath.Join(dir, ConfigDirName, ConfigFileName)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, dir, nil
		}
		legacy := filepath.Join(dir, LegacyConfigName)
		if _, statErr := os.Stat(legacy); statErr == nil {
			return legacy, dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", "", errors.New("could not find " + filepath.Join(ConfigDirName, ConfigFileName) + " or " + LegacyConfigName + " in this directory or any parent directory")
}
