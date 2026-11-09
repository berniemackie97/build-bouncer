package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const stateLockTimeout = 2 * time.Second

// LockStateFile serializes updates to a private state file across processes.
func LockStateFile(statePath string) (func() error, error) {
	lockPath := statePath + ".lock"
	parent := filepath.Dir(lockPath)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("state lock directory %q must not be a symbolic link or non-directory", parent)
	}
	return acquireStateFileLock(lockPath, os.Mkdir, time.Now().Add(stateLockTimeout))
}

func acquireStateFileLock(lockPath string, mkdir func(string, os.FileMode) error, deadline time.Time) (func() error, error) {
	for {
		err := mkdir(lockPath, 0o700)
		if err == nil {
			return func() error { return os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) && !(runtime.GOOS == "windows" && errors.Is(err, os.ErrPermission)) {
			return nil, fmt.Errorf("lock state file: %w", err)
		}
		if !time.Now().Before(deadline) {
			if errors.Is(err, os.ErrPermission) {
				return nil, fmt.Errorf("state file lock %q remains inaccessible; inspect lock and directory permissions: %w", lockPath, err)
			}
			return nil, fmt.Errorf("state file lock %q is busy or left by an interrupted process; inspect before removing it", lockPath)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
