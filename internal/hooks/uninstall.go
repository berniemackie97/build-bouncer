// Package hooks contains logic for installing and uninstalling git hooks used by build-bouncer.
package hooks

import (
	"errors"
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
	info, err := os.Lstat(hookPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("pre-push hook is not a regular file: %s", hookPath)
	}
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return err
	}
	ours := strings.Contains(string(data), prePushMarker)
	if !ours && !force {
		return fmt.Errorf("pre-push hook exists but was not installed by build-bouncer (use --force to remove)")
	}
	if err := removeFileWithRetries(hookPath); err != nil {
		return fmt.Errorf("remove hook: %w", err)
	}
	if !ours {
		return nil
	}

	p1, p2 := copiedBinaryPaths(hooksDir)
	var cleanup []error
	for _, path := range []string{p1, p2, filepath.Join(hooksDir, "bin", "build-bouncer.tmp"), filepath.Join(hooksDir, "bin", "build-bouncer.exe.tmp")} {
		if err := removeOwnedFile(path); err != nil {
			cleanup = append(cleanup, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	if err := removeDirIfEmpty(filepath.Join(hooksDir, "bin")); err != nil {
		cleanup = append(cleanup, fmt.Errorf("remove empty bin directory: %w", err))
	}
	return errors.Join(cleanup...)
}

func removeOwnedFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to remove non-regular file %q", path)
	}
	return removeFileWithRetries(path)
}

func removeDirIfEmpty(dir string) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing non-directory bin path %q", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
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
