package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LockStateFile serializes updates to a private state file across processes.
func LockStateFile(statePath string) (func() error, error) {
	lockPath := statePath + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := os.Mkdir(lockPath, 0o700); err == nil {
			return func() error { return os.Remove(lockPath) }, nil
		} else if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("lock state file: %w", err)
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("state file lock %q is busy or left by an interrupted process; inspect before removing it", lockPath)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
