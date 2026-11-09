package config

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithEditLockExcludesOtherProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := WithEditLock(path, func() error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestEditLockChild$")
		cmd.Env = append(os.Environ(), "BUILDBOUNCER_CONFIG_LOCK_TEST="+path)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return errors.New(string(output) + ": " + err.Error())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path + ".edit-lock"); !os.IsNotExist(err) {
		t.Fatalf("edit lock was not released: %v", err)
	}
	if err := WithEditLock(path, func() error { return nil }); err != nil {
		t.Fatalf("config lock could not be reused: %v", err)
	}
}

func TestEditLockChild(t *testing.T) {
	path := os.Getenv("BUILDBOUNCER_CONFIG_LOCK_TEST")
	if path == "" {
		return
	}
	called := false
	err := WithEditLock(path, func() error {
		called = true
		return nil
	})
	if err == nil || called || !strings.Contains(err.Error(), "edit lock exists") || !strings.Contains(err.Error(), filepath.Base(path)+".edit-lock") {
		t.Fatalf("other process entered edit lock: called=%t err=%v", called, err)
	}
}

func TestWithEditLockFailsClosedAndReleasesAfterError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	failure := errors.New("failed edit")
	if err := WithEditLock(path, func() error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("edit error was lost: %v", err)
	}
	lockPath := path + ".edit-lock"
	if _, err := os.Lstat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("failed edit retained the lock: %v", err)
	}
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	called := false
	err := WithEditLock(path, func() error { called = true; return nil })
	if err == nil || called || !strings.Contains(err.Error(), "inspect before removing") {
		t.Fatalf("stale lock bypassed: called=%t err=%v", called, err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := WithEditLock(path, func() error { return nil }); err != nil {
		t.Fatalf("recovered edit lock failed: %v", err)
	}
}
