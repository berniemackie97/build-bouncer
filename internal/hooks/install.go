package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/berniemackie97/build-bouncer/internal/storage"
)

type InstallOptions struct {
	CopySelf bool
	Force    bool
}

func InstallPrePushHook(opts InstallOptions) error {
	return installPrePushHook(opts, writePrePushHook)
}

func installPrePushHook(opts InstallOptions, publishHook func(string, []byte, bool) error) error {
	_, hooksDir, err := repoHooksDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	return withLifecycleLock(hooksDir, func() error {
		return installPrePushHookLocked(hooksDir, opts, publishHook)
	})
}

func installPrePushHookLocked(hooksDir string, opts InstallOptions, publishHook func(string, []byte, bool) error) error {
	hookPath := filepath.Join(hooksDir, "pre-push")
	previous, mode, existed, err := existingHook(hookPath, opts.Force)
	if err != nil {
		return err
	}

	body := []byte(renderPrePushHook(opts.CopySelf))
	if !opts.CopySelf {
		if err := publishHook(hookPath, body, existed); err != nil {
			return errors.Join(fmt.Errorf("install pre-push hook: %w", err),
				restorePublishedHook(hookPath, body, previous, mode, existed))
		}
		return nil
	}

	binDir := filepath.Join(hooksDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	defer func() { _ = removeDirIfEmpty(binDir) }()
	info, err := os.Lstat(binDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("copied executable directory is not a directory: %s", binDir)
	}

	binaryPath := hostCopiedBinaryPath(hooksDir)
	if err := validateCopiedBinaryDestination(binaryPath, opts.Force); err != nil {
		return err
	}
	previousProof, proofExisted, err := readCopiedBinaryProof(copiedBinaryProofPath(binaryPath))
	if err != nil {
		return err
	}

	stagedBinary, err := stageExecutable(binDir)
	if err != nil {
		return fmt.Errorf("stage copied binary: %w", err)
	}
	defer os.Remove(stagedBinary)
	copiedDigest, err := fileDigest(stagedBinary)
	if err != nil {
		return err
	}
	binaryInfo, err := os.Lstat(binaryPath)
	binaryExisted := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var backup string
	preserveBackup := false
	if binaryExisted {
		backup, err = stageFile(binaryPath, binDir, binaryInfo.Mode().Perm())
		if err != nil {
			return fmt.Errorf("backup copied binary: %w", err)
		}
		defer func() {
			if !preserveBackup {
				_ = os.Remove(backup)
			}
		}()
	}
	rollback := func() error {
		if err := restoreCopiedBinaryIfUnchanged(binaryPath, backup, binaryExisted, copiedDigest); err != nil {
			if binaryExisted {
				preserveBackup = true
				return fmt.Errorf("restore copied executable (original retained at %q): %w", backup, err)
			}
			return err
		}
		if err := restoreCopiedBinaryProof(binaryPath, previousProof, proofExisted); err != nil {
			return fmt.Errorf("restore copied executable ownership record: %w", err)
		}
		return nil
	}

	if err := storage.ReplaceStagedFile(stagedBinary, binaryPath); err != nil {
		return errors.Join(fmt.Errorf("install copied binary: %w", err), rollback())
	}
	if err := persistCopiedBinaryProof(binaryPath, copiedDigest); err != nil {
		return errors.Join(fmt.Errorf("record copied executable ownership: %w", err), rollback())
	}
	if err := publishHook(hookPath, body, existed); err != nil {
		return errors.Join(fmt.Errorf("install pre-push hook: %w", err),
			restorePublishedHook(hookPath, body, previous, mode, existed),
			rollback())
	}
	return nil
}

func writePrePushHook(path string, body []byte, existed bool) error {
	var err error
	if existed {
		err = storage.WriteFile(path, body, 0o755)
	} else {
		err = storage.CreateFile(path, body, 0o755)
	}
	if err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

func restoreCopiedBinaryIfUnchanged(path, backup string, existed bool, publishedDigest string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if !existed {
			return nil
		}
		return fmt.Errorf("copied executable disappeared during installation: %s", path)
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("copied executable changed type during installation: %s", path)
	}
	actual, err := fileDigest(path)
	if err != nil {
		return err
	}
	if actual != publishedDigest {
		if existed {
			original, err := fileDigest(backup)
			if err != nil {
				return err
			}
			if actual == original {
				return nil
			}
		}
		return fmt.Errorf("copied executable changed during installation; refusing to overwrite: %s", path)
	}
	if !existed {
		return removeFileWithRetries(path)
	}
	return storage.ReplaceStagedFile(backup, path)
}

func existingHook(path string, force bool) ([]byte, os.FileMode, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("pre-push hook is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	if !knownGeneratedPrePushHook(data) && !force {
		if ownedPrePushHook(data) {
			return nil, 0, false, fmt.Errorf("pre-push hook has modified or older build-bouncer content (inspect it and use --force to overwrite)")
		}
		return nil, 0, false, fmt.Errorf("pre-push hook exists but was not installed by build-bouncer (use --force to overwrite)")
	}
	return data, info.Mode().Perm(), true, nil
}

func restorePublishedHook(path string, published, previous []byte, mode os.FileMode, existed bool) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) && !existed {
		return nil
	}
	if err != nil {
		return err
	}
	if existed && bytes.Equal(data, previous) {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode().Perm() != mode {
			return os.Chmod(path, mode)
		}
		return nil
	}
	if !bytes.Equal(data, published) {
		return fmt.Errorf("pre-push hook changed during installation; refusing to overwrite: %s", path)
	}
	return restoreHook(path, previous, mode, existed)
}

func restoreHook(path string, previous []byte, mode os.FileMode, existed bool) error {
	if !existed {
		return removeFileWithRetries(path)
	}
	if err := storage.WriteFile(path, previous, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func stageExecutable(dir string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return stageFile(executable, dir, 0o755)
}

func stageFile(source, dir string, mode os.FileMode) (string, error) {
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.CreateTemp(dir, ".build-bouncer-*.tmp")
	if err != nil {
		return "", err
	}
	defer func() {
		if out != nil {
			_ = out.Close()
			_ = os.Remove(out.Name())
		}
	}()
	if err := out.Chmod(mode); err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	if err := out.Sync(); err != nil {
		return "", err
	}
	path := out.Name()
	if err := out.Close(); err != nil {
		return "", err
	}
	out = nil
	return path, nil
}
