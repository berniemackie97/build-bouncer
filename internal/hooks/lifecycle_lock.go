package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const lifecycleLockName = ".build-bouncer-hook.lock"

func withLifecycleLock(hooksDir string, action func() error) (err error) {
	lockPath := filepath.Join(hooksDir, lifecycleLockName)
	if createErr := os.Mkdir(lockPath, 0o700); createErr != nil {
		if errors.Is(createErr, os.ErrExist) {
			return lifecycleLockError(lockPath)
		}
		return fmt.Errorf("acquire hook lifecycle lock: %w", createErr)
	}
	defer func() {
		if releaseErr := os.Remove(lockPath); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release hook lifecycle lock %q: %w", lockPath, releaseErr))
		}
	}()
	return action()
}

func ensureLifecycleUnlocked(hooksDir string) error {
	lockPath := filepath.Join(hooksDir, lifecycleLockName)
	if _, err := os.Lstat(lockPath); err == nil {
		return lifecycleLockError(lockPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect hook lifecycle lock: %w", err)
	}
	return nil
}

func lifecycleLockError(path string) error {
	return fmt.Errorf("hook lifecycle lock exists at %q; another operation may be running or an earlier one was interrupted (inspect before removing the lock)", path)
}
