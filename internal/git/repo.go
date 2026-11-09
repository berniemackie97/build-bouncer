// Package git contains small helpers for discovering Git repository context
// without shelling out to the `git` binary.
package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func FindRepoRootOrCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if root, ok := findRepoRootFrom(cwd); ok {
		return root, nil
	}

	return cwd, nil
}

func FindRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if root, ok := findRepoRootFrom(cwd); ok {
		return root, nil
	}

	return "", errors.New("not inside a git repository (no valid .git found)")
}

func findRepoRootFrom(start string) (string, bool) {
	dir := start
	for {
		if _, ok := ResolveGitDir(dir); ok {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// ResolveGitDir locates the metadata directory for a repository checkout.
// Linked worktrees and submodules store the metadata location in a .git file.
func ResolveGitDir(repoRoot string) (string, bool) {
	marker := filepath.Join(repoRoot, ".git")
	info, err := os.Lstat(marker)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return marker, true
	}
	if !info.Mode().IsRegular() {
		return "", false
	}

	data, err := readFilePrefix(marker, 4096)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		const prefix = "gitdir:"
		if len(line) < len(prefix) || !strings.EqualFold(line[:len(prefix)], prefix) {
			return "", false
		}
		target := strings.TrimSpace(line[len(prefix):])
		if target == "" {
			return "", false
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(repoRoot, target)
		}
		target = filepath.Clean(target)
		info, err := os.Stat(target)
		if err != nil || !info.IsDir() {
			return "", false
		}
		return target, true
	}
	return "", false
}

func readFilePrefix(path string, maxBytes int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, maxBytes)
	n, readErr := f.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	return nil, readErr
}
