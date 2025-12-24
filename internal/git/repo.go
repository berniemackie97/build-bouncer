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
		candidate := filepath.Join(dir, ".git")
		if isGitMarker(candidate) {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// isGitMarker returns true if path is either:
//   - a .git directory (normal repo), OR
//   - a .git file that looks like a worktree/submodule pointer ("gitdir: <path>").
func isGitMarker(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	if st.IsDir() {
		return true
	}
	if !st.Mode().IsRegular() {
		return false
	}

	// Worktrees/submodules store a small text file like:
	//   gitdir: /actual/path/to/git/dir
	// Read only a small prefix to validate.
	const maxRead = 4096
	b, err := readFilePrefix(path, maxRead)
	if err != nil {
		return false
	}

	text := strings.TrimSpace(string(b))
	if text == "" {
		return false
	}

	// Only consider the first non-empty line.
	lines := strings.Split(text, "\n")
	first := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line != "" {
			first = line
			break
		}
	}
	if first == "" {
		return false
	}

	lower := strings.ToLower(first)
	const prefix = "gitdir:"
	if !strings.HasPrefix(lower, prefix) {
		return false
	}

	rest := strings.TrimSpace(first[len(prefix):])
	return rest != ""
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
