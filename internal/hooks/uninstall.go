// Package hooks contains logic for installing and uninstalling git hooks used by build-bouncer.
package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func UninstallPrePushHook(force bool) error {
	_, hooksDir, err := repoHooksDir()
	if err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "pre-push")

	// If the hook exists, only remove it when it is ours, unless forced.
	if b, readErr := os.ReadFile(hookPath); readErr == nil {
		isOurs := strings.Contains(string(b), prePushMarker)
		if !isOurs && !force {
			return fmt.Errorf("pre-push hook exists but was not installed by build-bouncer (use --force to remove)")
		}

		// Use retry logic for Windows to handle file locks
		if err := removeFileWithRetries(hookPath); err != nil {
			return fmt.Errorf("remove hook: %w", err)
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}

	// Clean up copied binaries with retry logic
	p1, p2 := copiedBinaryPaths(hooksDir)
	if err := removeFileWithRetries(p1); err != nil && !os.IsNotExist(err) {
		// Log but don't fail on binary cleanup errors
		_ = err
	}
	if p2 != p1 {
		if err := removeFileWithRetries(p2); err != nil && !os.IsNotExist(err) {
			// Log but don't fail on binary cleanup errors
			_ = err
		}
	}

	// Clean up temporary files that may have been left behind
	binDir := filepath.Join(hooksDir, "bin")
	cleanupTempFiles(binDir)

	// Remove bin dir if it exists and is empty
	if err := removeDirIfEmpty(binDir); err != nil {
		// Log but don't fail on directory cleanup errors
		_ = err
	}

	return nil
}

// cleanupTempFiles removes any .tmp files left behind in the hooks bin directory
func cleanupTempFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tmp") {
			tmpPath := filepath.Join(dir, entry.Name())
			_ = removeFileWithRetries(tmpPath)
		}
	}
}

func removeDirIfEmpty(dir string) error {
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !st.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		// If we can't read it, we can't safely decide it's empty.
		return err
	}
	if len(entries) != 0 {
		return nil
	}

	if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
