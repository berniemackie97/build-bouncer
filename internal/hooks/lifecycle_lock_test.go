package hooks

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleLockBlocksConcurrentProcesses(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	if err := InstallPrePushHook(InstallOptions{CopySelf: false}); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(hooksDir, "pre-push")
	original, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Dir(filepath.Dir(hooksDir))
	if err := withLifecycleLock(hooksDir, func() error {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		cmd := exec.Command(self, "-test.run=^TestLifecycleLockChild$")
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "BUILDBOUNCER_HOOK_LOCK_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("other process did not refuse locked hook: %w: %s", err, output)
		}
		if _, err := GetStatus(); err == nil || !strings.Contains(err.Error(), "hook lifecycle lock") {
			return fmt.Errorf("hook status returned a transient installation snapshot: %v", err)
		}
		if err := UninstallPrePushHook(true); err == nil || !strings.Contains(err.Error(), "hook lifecycle lock") {
			return fmt.Errorf("forced uninstall ignored lifecycle lock: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertLifecycleArtifact(t, hookPath, original)
	if err := UninstallPrePushHook(false); err != nil {
		t.Fatalf("uninstall after release failed: %v", err)
	}
}

func TestLifecycleLockChild(t *testing.T) {
	if os.Getenv("BUILDBOUNCER_HOOK_LOCK_CHILD") != "1" {
		return
	}
	for _, operation := range []func() error{
		func() error { return InstallPrePushHook(InstallOptions{CopySelf: true, Force: true}) },
		func() error { return UninstallPrePushHook(true) },
	} {
		if err := operation(); err == nil || !strings.Contains(err.Error(), "hook lifecycle lock") {
			t.Fatalf("cross-process lock not enforced: %v", err)
		}
	}
}

func TestLifecycleLockRequiresManualRecoveryAfterInterruption(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	lockPath := filepath.Join(hooksDir, lifecycleLockName)
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func() error{
		func() error { return InstallPrePushHook(InstallOptions{CopySelf: false, Force: true}) },
		func() error { return UninstallPrePushHook(true) },
		func() error { _, err := GetStatus(); return err },
	} {
		if err := operation(); err == nil ||
			!strings.Contains(err.Error(), "hook lifecycle lock") ||
			!strings.Contains(err.Error(), lifecycleLockName) {
			t.Fatalf("stale lock should block safely with useful location: %v", err)
		}
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := InstallPrePushHook(InstallOptions{CopySelf: false}); err != nil {
		t.Fatalf("recovery after manual lock removal: %v", err)
	}
	if _, err := os.Lstat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("successful install left a lock: %v", err)
	}
}

func TestLifecycleLockReleasedAfterFailedInstallation(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	failure := errors.New("injected hook failure")
	err := installPrePushHook(InstallOptions{CopySelf: true}, func(string, []byte, bool) error {
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("injected error not returned: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(hooksDir, lifecycleLockName)); !os.IsNotExist(err) {
		t.Fatalf("failed install kept lifecycle lock: %v", err)
	}
	if err := InstallPrePushHook(InstallOptions{CopySelf: true}); err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
}
