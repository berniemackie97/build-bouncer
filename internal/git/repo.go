package git

import (
	"errors"
	"os"
	"path/filepath"
)

func FindRepoRootOrCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if root, ok := findUp(cwd, ".git"); ok {
		return root, nil
	}

	return cwd, nil
}

func FindRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if root, ok := findUp(cwd, ".git"); ok {
		return root, nil
	}

	return "", errors.New("not inside a git repository (no .git found)")
}

func findUp(start string, target string) (string, bool) {
	dir := start
	for {
		candidate := filepath.Join(dir, target)
		if _, err := os.Stat(candidate); err == nil {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
