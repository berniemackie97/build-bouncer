// Package hooks contains logic for installing and uninstalling git hooks used by build-bouncer.
package hooks

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
	ours := ownedPrePushHook(data)
	if !generatedPrePushHook(data) && !force {
		if ours {
			return fmt.Errorf("pre-push hook has modified or older build-bouncer content (inspect it and use --force to remove)")
		}
		return fmt.Errorf("pre-push hook exists but was not installed by build-bouncer (use --force to remove)")
	}
	if !ours {
		return removeFileWithRetries(hookPath)
	}
	if !bytes.Equal(data, []byte(renderPrePushHook(true))) {
		return removeFileWithRetries(hookPath)
	}

	binDir := filepath.Join(hooksDir, "bin")
	if info, err := os.Lstat(binDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("refusing non-directory copied executable path %q", binDir)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	binaryPath := hostCopiedBinaryPath(hooksDir)
	exists, owned, err := copiedBinaryOwnership(binaryPath)
	if err != nil {
		return err
	}
	if exists && !owned && !force {
		return fmt.Errorf("copied executable has no matching ownership record: %s (inspect it and use --force to remove)", binaryPath)
	}
	if exists {
		if err := removeOwnedFile(binaryPath); err != nil {
			return fmt.Errorf("remove copied executable: %w", err)
		}
		if err := removeOwnedFile(copiedBinaryProofPath(binaryPath)); err != nil {
			return fmt.Errorf("remove copied executable ownership record: %w", err)
		}
	}
	if err := removeFileWithRetries(hookPath); err != nil {
		return fmt.Errorf("remove hook: %w", err)
	}
	return removeDirIfEmpty(binDir)
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
