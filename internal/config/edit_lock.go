package config

import (
	"errors"
	"fmt"
	"os"
)

func WithEditLock(path string, edit func() error) (err error) {
	lockPath := path + ".edit-lock"
	if acquireErr := os.Mkdir(lockPath, 0o700); acquireErr != nil {
		if errors.Is(acquireErr, os.ErrExist) {
			return fmt.Errorf("configuration edit lock exists at %q; another operation may be running or an earlier one was interrupted (inspect before removing the lock)", lockPath)
		}
		return fmt.Errorf("acquire configuration edit lock: %w", acquireErr)
	}
	defer func() {
		if releaseErr := os.Remove(lockPath); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release configuration edit lock %q: %w", lockPath, releaseErr))
		}
	}()
	return edit()
}
