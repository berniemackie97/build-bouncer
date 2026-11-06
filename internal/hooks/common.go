package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/git"
)

const prePushMarker = "# build-bouncer pre-push hook v"

func repoHooksDir() (repoRoot string, hooksDir string, err error) {
	root, err := git.FindRepoRoot()
	if err != nil {
		return "", "", err
	}

	output, err := exec.Command("git", "-C", root, "rev-parse", "--git-path", "hooks").Output()
	if err != nil {
		return "", "", fmt.Errorf("resolve Git hooks directory: %w", err)
	}
	hooks := strings.TrimSpace(string(output))
	if hooks == "" {
		return "", "", fmt.Errorf("Git returned an empty hooks path")
	}
	if !filepath.IsAbs(hooks) {
		hooks = filepath.Join(root, hooks)
	}
	return root, filepath.Clean(hooks), nil
}

func copiedBinaryPaths(hooksDir string) (string, string) {
	base := filepath.Join(hooksDir, "bin", "build-bouncer")
	return base, base + ".exe"
}

// removeFileWithRetries attempts to remove a file with retry logic for Windows file locking.
func removeFileWithRetries(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	// Non-Windows: quick attempt is fine.
	if runtime.GOOS != "windows" {
		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		return err
	}

	const attempts = 12
	const delay = 15 * time.Millisecond

	var lastErr error
	for range attempts {
		lastErr = os.Remove(path)
		if lastErr == nil || os.IsNotExist(lastErr) {
			return nil
		}
		time.Sleep(delay)
	}
	return lastErr
}
