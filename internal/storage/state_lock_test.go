package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStateLockConcurrentAcquisitionPreservesExclusion(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "history.json")
	var active, completed atomic.Int32
	var overlapped atomic.Bool
	var group sync.WaitGroup
	errs := make(chan error, 6)

	for range 6 {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 10 {
				unlock, err := LockStateFile(statePath)
				if err != nil {
					errs <- err
					return
				}
				if active.Add(1) != 1 {
					overlapped.Store(true)
				}
				time.Sleep(time.Millisecond)
				completed.Add(1)
				active.Add(-1)
				if err := unlock(); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent state lock: %v", err)
	}
	if overlapped.Load() || completed.Load() != 60 {
		t.Fatalf("state lock lost exclusion or progress: overlapped=%v completed=%d", overlapped.Load(), completed.Load())
	}
	if _, err := os.Lstat(statePath + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state lock survived after all writers finished: %v", err)
	}
}

func TestStateLockClassifiesTransientWindowsPermissionError(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "history.json.lock")
	attempts := 0
	create := func(path string, mode os.FileMode) error {
		attempts++
		if attempts == 1 {
			return &os.PathError{Op: "mkdir", Path: path, Err: os.ErrPermission}
		}
		return os.Mkdir(path, mode)
	}
	unlock, err := acquireStateFileLock(lockPath, create, time.Now().Add(time.Second))
	if runtime.GOOS != "windows" {
		if !errors.Is(err, os.ErrPermission) || attempts != 1 || unlock != nil {
			t.Fatalf("unexpected POSIX permission retry: attempts=%d err=%v", attempts, err)
		}
		return
	}
	if err != nil || attempts != 2 || unlock == nil {
		t.Fatalf("Windows transient permission error was not retried: attempts=%d err=%v", attempts, err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestStateLockRetainsPermanentPermissionFailure(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "history.json.lock")
	attempts := 0
	create := func(path string, mode os.FileMode) error {
		attempts++
		return &os.PathError{Op: "mkdir", Path: path, Err: os.ErrPermission}
	}
	unlock, err := acquireStateFileLock(lockPath, create, time.Now().Add(-time.Second))
	if !errors.Is(err, os.ErrPermission) || attempts != 1 || unlock != nil {
		t.Fatalf("permanent permission denial lost: attempts=%d err=%v", attempts, err)
	}
	if _, err := os.Lstat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("denied lock was created or modified: %v", err)
	}
}

func TestStateLockPreservesBusyLockRecoveryGuidance(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "history.json.lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	unlock, err := acquireStateFileLock(lockPath, os.Mkdir, time.Now().Add(-time.Second))
	if unlock != nil || err == nil || !strings.Contains(err.Error(), "inspect before removing it") {
		t.Fatalf("busy lock bypassed: %v", err)
	}
	if info, err := os.Lstat(lockPath); err != nil || !info.IsDir() {
		t.Fatalf("busy lock was removed: %v", err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	unlock, err = LockStateFile(strings.TrimSuffix(lockPath, ".lock"))
	if err != nil || unlock == nil {
		t.Fatalf("lock could not recover after removal: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestStateLockRefusesSymbolicLinkParent(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	unlock, err := LockStateFile(filepath.Join(link, "history.json"))
	if err == nil || unlock != nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("state lock followed untrusted link parent: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "history.json.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock created through symbolic link: %v", err)
	}
}
