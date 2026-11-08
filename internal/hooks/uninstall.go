// Package hooks contains logic for installing and uninstalling git hooks used by build-bouncer.
package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/berniemackie97/build-bouncer/internal/storage"
)

func UninstallPrePushHook(force bool) error {
	return uninstallPrePushHook(force, removeFileWithRetries)
}

func uninstallPrePushHook(force bool, removeHook func(string) error) error {
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
	if !knownGeneratedPrePushHook(data) && !force {
		if ours {
			return fmt.Errorf("pre-push hook has modified or older build-bouncer content (inspect it and use --force to remove)")
		}
		return fmt.Errorf("pre-push hook exists but was not installed by build-bouncer (use --force to remove)")
	}
	if !ours {
		return removeHook(hookPath)
	}
	copied := bytes.Equal(data, []byte(renderPrePushHook(true)))
	legacyCopied := bytes.Equal(data, []byte(renderLegacyPrePushHook(true)))
	if !copied && !legacyCopied {
		return removeHook(hookPath)
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
	if legacyCopied {
		binaryPath = hostLegacyCopiedBinaryPath(hooksDir)
	}
	exists, owned, err := copiedBinaryOwnership(binaryPath)
	if err != nil {
		return err
	}
	if exists && !owned && !force {
		return fmt.Errorf("copied executable has no matching ownership record: %s (inspect it and use --force to remove)", binaryPath)
	}
	var backup string
	var proof string
	var recorded bool
	if exists {
		binaryInfo, err := os.Lstat(binaryPath)
		if err != nil {
			return err
		}
		backup, err = stageFile(binaryPath, binDir, binaryInfo.Mode().Perm())
		if err != nil {
			return fmt.Errorf("backup copied executable: %w", err)
		}
		defer os.Remove(backup)
		proof, recorded, err = readCopiedBinaryProof(copiedBinaryProofPath(binaryPath))
		if err != nil {
			return err
		}
		if err := removeOwnedFile(binaryPath); err != nil {
			return fmt.Errorf("remove copied executable: %w", err)
		}
		if err := removeOwnedFile(copiedBinaryProofPath(binaryPath)); err != nil {
			return errors.Join(fmt.Errorf("remove copied executable ownership record: %w", err),
				os.Link(backup, binaryPath))
		}
	}
	if err := removeHook(hookPath); err != nil {
		if exists {
			return errors.Join(fmt.Errorf("remove hook: %w", err),
				restoreDeletedHook(hookPath, data, info.Mode().Perm()),
				os.Link(backup, binaryPath),
				restoreCopiedBinaryProof(binaryPath, proof, recorded))
		}
		return errors.Join(fmt.Errorf("remove hook: %w", err),
			restoreDeletedHook(hookPath, data, info.Mode().Perm()))
	}
	if backup != "" {
		if err := removeFileWithRetries(backup); err != nil {
			return fmt.Errorf("remove staged copied executable backup: %w", err)
		}
	}
	return removeDirIfEmpty(binDir)
}

func restoreDeletedHook(path string, previous []byte, mode os.FileMode) error {
	data, err := os.ReadFile(path)
	if err == nil {
		if bytes.Equal(data, previous) {
			return nil
		}
		return fmt.Errorf("pre-push hook changed during removal; refusing to overwrite: %s", path)
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := storage.CreateFile(path, previous, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
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
