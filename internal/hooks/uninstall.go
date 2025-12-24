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

		if removeErr := os.Remove(hookPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}

	// Best-effort cleanup of copied binary (we only touch known target paths).
	p1, p2 := copiedBinaryPaths(hooksDir)
	_ = os.Remove(p1)
	if p2 != p1 {
		_ = os.Remove(p2)
	}

	// Remove bin dir if it exists and is empty.
	binDir := filepath.Join(hooksDir, "bin")
	_ = removeDirIfEmpty(binDir)

	return nil
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
