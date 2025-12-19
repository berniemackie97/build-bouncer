package config

import (
	"errors"
	"os"
	"path/filepath"
)

func FindConfigFromCwd(filename string) (cfgPath string, cfgDir string, err error) {
	start, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	dir := start
	for {
		candidate := filepath.Join(dir, filename)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", "", errors.New("could not find " + filename + " in this directory or any parent directory")
}
