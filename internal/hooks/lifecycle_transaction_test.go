package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func newLifecycleRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if output, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	return filepath.Join(root, ".git", "hooks")
}

func assertLifecycleArtifact(t *testing.T, path string, want []byte) {
	t.Helper()
	actual, err := os.ReadFile(path)
	if want == nil {
		if !os.IsNotExist(err) {
			t.Fatalf("expected %s absent, got %v", path, err)
		}
		return
	}
	if err != nil || !bytes.Equal(actual, want) {
		t.Fatalf("artifact %s changed unexpectedly: %v", path, err)
	}
}

func assertNoStagedFiles(t *testing.T, binDir string) {
	t.Helper()
	entries, err := os.ReadDir(binDir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".build-bouncer-") {
			t.Fatalf("staged file leaked: %s", entry.Name())
		}
	}
}

func TestInstallRollbackOnHookPublicationFailure(t *testing.T) {
	for _, variant := range []string{"new install", "reinstall", "force foreign hook and binary"} {
		t.Run(variant, func(t *testing.T) {
			hooksDir := newLifecycleRepo(t)
			hookPath := filepath.Join(hooksDir, "pre-push")
			binary := hostCopiedBinaryPath(hooksDir)
			proof := copiedBinaryProofPath(binary)
			switch variant {
			case "reinstall":
				if err := InstallPrePushHook(InstallOptions{CopySelf: true}); err != nil {
					t.Fatal(err)
				}
			case "force foreign hook and binary":
				if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 41\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(binary, []byte("foreign binary"), 0o700); err != nil {
					t.Fatal(err)
				}
			}

			beforeHook, _ := os.ReadFile(hookPath)
			beforeBinary, _ := os.ReadFile(binary)
			beforeProof, _ := os.ReadFile(proof)
			failure := errors.New("injected publication failure")
			published := false
			err := installPrePushHook(InstallOptions{
				CopySelf: true,
				Force:    variant == "force foreign hook and binary",
			}, func(path string, body []byte, existed bool) error {
				if err := writePrePushHook(path, body, existed); err != nil {
					return err
				}
				published = true
				return failure
			})
			if !published || !errors.Is(err, failure) {
				t.Fatalf("did not exercise post-publication failure: %v", err)
			}
			assertLifecycleArtifact(t, hookPath, beforeHook)
			assertLifecycleArtifact(t, binary, beforeBinary)
			assertLifecycleArtifact(t, proof, beforeProof)
			assertNoStagedFiles(t, filepath.Dir(binary))
		})
	}
}

func TestInstallRollbackDoesNotRemoveConcurrentForeignHook(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	hookPath := filepath.Join(hooksDir, "pre-push")
	binary := hostCopiedBinaryPath(hooksDir)
	foreign := []byte("#!/bin/sh\necho teammate\n")
	failure := errors.New("injected concurrent creation")
	err := installPrePushHook(InstallOptions{CopySelf: true},
		func(path string, _ []byte, _ bool) error {
			if err := os.WriteFile(path, foreign, 0o700); err != nil {
				return err
			}
			return failure
		})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "changed during installation") {
		t.Fatalf("unsafe concurrent hook handling: %v", err)
	}
	assertLifecycleArtifact(t, hookPath, foreign)
	assertLifecycleArtifact(t, binary, nil)
	assertLifecycleArtifact(t, copiedBinaryProofPath(binary), nil)
	assertNoStagedFiles(t, filepath.Dir(binary))
}

func TestUninstallRollbackOnHookRemovalFailure(t *testing.T) {
	for _, afterRemoval := range []bool{false, true} {
		name := "before removal"
		if afterRemoval {
			name = "after removal"
		}
		t.Run(name, func(t *testing.T) {
			hooksDir := newLifecycleRepo(t)
			if err := InstallPrePushHook(InstallOptions{CopySelf: true}); err != nil {
				t.Fatal(err)
			}
			hookPath := filepath.Join(hooksDir, "pre-push")
			binary := hostCopiedBinaryPath(hooksDir)
			proof := copiedBinaryProofPath(binary)
			originalHook, _ := os.ReadFile(hookPath)
			originalBinary, _ := os.ReadFile(binary)
			originalProof, _ := os.ReadFile(proof)
			failure := errors.New("injected hook deletion failure")
			called := false
			err := uninstallPrePushHook(false, func(path string) error {
				called = true
				if afterRemoval {
					if err := removeFileWithRetries(path); err != nil {
						return err
					}
				}
				return failure
			})
			if !called || !errors.Is(err, failure) {
				t.Fatalf("did not exercise post-cleanup failure: %v", err)
			}
			assertLifecycleArtifact(t, hookPath, originalHook)
			assertLifecycleArtifact(t, binary, originalBinary)
			assertLifecycleArtifact(t, proof, originalProof)
			assertNoStagedFiles(t, filepath.Dir(binary))
		})
	}
}

func TestLegacyV3HookUpgradeAndCleanup(t *testing.T) {
	for _, copied := range []bool{false, true} {
		name := "PATH only"
		if copied {
			name = "copied binary"
		}
		t.Run(name, func(t *testing.T) {
			hooksDir := newLifecycleRepo(t)
			hookPath := filepath.Join(hooksDir, "pre-push")
			oldHook := []byte(renderLegacyPrePushHook(copied))
			if err := os.WriteFile(hookPath, oldHook, 0o755); err != nil {
				t.Fatal(err)
			}
			legacyPath := hostLegacyCopiedBinaryPath(hooksDir)
			var legacyCopy []byte
			if copied {
				if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
					t.Fatal(err)
				}
				legacyCopy = []byte("v3 executable preserved for other worktrees")
				if err := os.WriteFile(legacyPath, legacyCopy, 0o755); err != nil {
					t.Fatal(err)
				}
				digest, err := fileDigest(legacyPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := persistCopiedBinaryProof(legacyPath, digest); err != nil {
					t.Fatal(err)
				}
			}
			before, err := GetStatus()
			if err != nil || !before.Ours || !before.LegacyGenerated || before.Current {
				t.Fatalf("pristine v3 hook not identified as upgradeable: %+v %v", before, err)
			}
			if err := InstallPrePushHook(InstallOptions{CopySelf: copied}); err != nil {
				t.Fatalf("pristine v3 must upgrade unforced: %v", err)
			}
			after, err := GetStatus()
			if err != nil || !after.Current || after.LegacyGenerated {
				t.Fatalf("v4 hook not recognized after upgrade: %+v %v", after, err)
			}
			assertLifecycleArtifact(t, hookPath, []byte(renderPrePushHook(copied)))
			if copied {
				assertLifecycleArtifact(t, legacyPath, legacyCopy)
				if _, err := os.Stat(hostCopiedBinaryPath(hooksDir)); err != nil {
					t.Fatalf("isolated v4 executable missing: %v", err)
				}
			}
			if err := UninstallPrePushHook(false); err != nil {
				t.Fatalf("v4 uninstall: %v", err)
			}
			assertLifecycleArtifact(t, hookPath, nil)
			if copied {
				assertLifecycleArtifact(t, legacyPath, legacyCopy)
				if err := os.WriteFile(hookPath, oldHook, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := UninstallPrePushHook(false); err != nil {
					t.Fatalf("uninstall pristine legacy hook: %v", err)
				}
				assertLifecycleArtifact(t, legacyPath, nil)
				assertLifecycleArtifact(t, copiedBinaryProofPath(legacyPath), nil)
			}
		})
	}
}

func TestModifiedV3HookStillRequiresForce(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	hookPath := filepath.Join(hooksDir, "pre-push")
	modified := append([]byte(renderLegacyPrePushHook(false)), []byte("# custom behavior\n")...)
	if err := os.WriteFile(hookPath, modified, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, op := range []func() error{
		func() error { return InstallPrePushHook(InstallOptions{CopySelf: false}) },
		func() error { return UninstallPrePushHook(false) },
	} {
		if err := op(); err == nil || !strings.Contains(err.Error(), "modified or older") {
			t.Fatalf("modified v3 should be refused without force: %v", err)
		}
		assertLifecycleArtifact(t, hookPath, modified)
	}
}

func TestV4CopiedHookBlocksRealGitPush(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executable invocation is covered by the mixed-workspace Git-push smoke")
	}
	hooksDir := newLifecycleRepo(t)
	repo := filepath.Dir(filepath.Dir(hooksDir))
	bare := filepath.Join(t.TempDir(), "remote.git")
	for _, args := range [][]string{
		{"init", "-q", "--bare", bare},
		{"-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com",
			"commit", "--allow-empty", "-m", "initial"},
		{"-C", repo, "remote", "add", "origin", bare},
	} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	binary := hostCopiedBinaryPath(hooksDir)
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf checked > .hook-invoked\nexit 17\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(hooksDir, "pre-push")
	if err := os.WriteFile(hookPath, []byte(renderPrePushHook(true)), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "-C", repo, "push", "origin", "HEAD:refs/heads/verification")
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("push unexpectedly succeeded despite failing copied hook: %s", output)
	}
	assertLifecycleArtifact(t, filepath.Join(repo, ".hook-invoked"), []byte("checked"))
}

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
		if err := operation(); err == nil || !strings.Contains(err.Error(), lockPath) {
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

func TestInstallRollbackPreservesConcurrentBinaryAndOriginalBackup(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	if err := InstallPrePushHook(InstallOptions{CopySelf: true}); err != nil {
		t.Fatal(err)
	}
	binary := hostCopiedBinaryPath(hooksDir)
	originalDigest, err := fileDigest(binary)
	if err != nil {
		t.Fatal(err)
	}
	foreign := []byte("concurrently replaced by another program")
	failure := errors.New("injected failure after binary publication")
	err = installPrePushHook(InstallOptions{CopySelf: true}, func(string, []byte, bool) error {
		if err := os.WriteFile(binary, foreign, 0o755); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "original retained at") {
		t.Fatalf("unsafe copied binary rollback: %v", err)
	}
	assertLifecycleArtifact(t, binary, foreign)
	assertBackupRetained(t, filepath.Dir(binary), originalDigest)
}

func TestUninstallRollbackPreservesConcurrentBinaryAndOriginalBackup(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	if err := InstallPrePushHook(InstallOptions{CopySelf: true}); err != nil {
		t.Fatal(err)
	}
	binary := hostCopiedBinaryPath(hooksDir)
	originalDigest, err := fileDigest(binary)
	if err != nil {
		t.Fatal(err)
	}
	foreign := []byte("concurrent replacement during uninstall")
	failure := errors.New("injected removal failure")
	err = uninstallPrePushHook(false, func(string) error {
		return errors.Join(os.WriteFile(binary, foreign, 0o755), failure)
	})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "original retained at") {
		t.Fatalf("unsafe copied binary rollback: %v", err)
	}
	assertLifecycleArtifact(t, binary, foreign)
	assertBackupRetained(t, filepath.Dir(binary), originalDigest)
}

func assertBackupRetained(t *testing.T, dir, originalDigest string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".build-bouncer-") {
			continue
		}
		digest, err := fileDigest(filepath.Join(dir, entry.Name()))
		if err == nil && digest == originalDigest {
			return
		}
	}
	t.Fatal("original copied executable backup was not retained after refused recovery")
}

func TestOrphanedOwnershipRecordRequiresForceForRecovery(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	if err := InstallPrePushHook(InstallOptions{CopySelf: true}); err != nil {
		t.Fatal(err)
	}
	binary := hostCopiedBinaryPath(hooksDir)
	proof := copiedBinaryProofPath(binary)
	hook := filepath.Join(hooksDir, "pre-push")
	before, err := os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	st, err := GetStatus()
	if err != nil || !st.CopiedBinaryRecordOrphaned || st.CopiedBinaryMatchesRecord {
		t.Fatalf("missing copied executable orphan not detected: %+v %v", st, err)
	}
	for _, action := range []func() error{
		func() error { return InstallPrePushHook(InstallOptions{CopySelf: true}) },
		func() error { return UninstallPrePushHook(false) },
	} {
		if err := action(); err == nil || !strings.Contains(err.Error(), "ownership record exists without its executable") {
			t.Fatalf("orphan should block implicit repair: %v", err)
		}
		assertLifecycleArtifact(t, hook, before)
	}
	if err := InstallPrePushHook(InstallOptions{CopySelf: true, Force: true}); err != nil {
		t.Fatalf("explicit repair of valid orphaned record: %v", err)
	}
	_, owned, err := copiedBinaryOwnership(binary)
	if err != nil || !owned {
		t.Fatalf("copy not repaired: %v", err)
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if err := UninstallPrePushHook(true); err != nil {
		t.Fatalf("explicit cleanup of valid orphan: %v", err)
	}
	assertLifecycleArtifact(t, proof, nil)
	assertLifecycleArtifact(t, hook, nil)
}

func TestInvalidOrphanedRecordRefusesForcedRecovery(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	if err := InstallPrePushHook(InstallOptions{CopySelf: true}); err != nil {
		t.Fatal(err)
	}
	binary := hostCopiedBinaryPath(hooksDir)
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	invalid := []byte("invalid ownership record")
	if err := os.WriteFile(copiedBinaryProofPath(binary), invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []func() error{
		func() error { return InstallPrePushHook(InstallOptions{CopySelf: true, Force: true}) },
		func() error { return UninstallPrePushHook(true) },
	} {
		if err := action(); err == nil || !strings.Contains(err.Error(), "invalid copied executable ownership record") {
			t.Fatalf("malformed ownership record should be protected: %v", err)
		}
		assertLifecycleArtifact(t, copiedBinaryProofPath(binary), invalid)
	}
}

func TestOrphanedRecordCleanupRestoresHookOnRemovalFailure(t *testing.T) {
	hooksDir := newLifecycleRepo(t)
	if err := InstallPrePushHook(InstallOptions{CopySelf: true}); err != nil {
		t.Fatal(err)
	}
	binary := hostCopiedBinaryPath(hooksDir)
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(hooksDir, "pre-push")
	previous, err := os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	proof := copiedBinaryProofPath(binary)
	record, err := os.ReadFile(proof)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("failure after deleting hook")
	err = uninstallPrePushHook(true, func(path string) error {
		if err := removeFileWithRetries(path); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("injected removal error missing: %v", err)
	}
	assertLifecycleArtifact(t, hook, previous)
	assertLifecycleArtifact(t, proof, record)
	if err := UninstallPrePushHook(true); err != nil {
		t.Fatalf("forced orphan cleanup after rollback: %v", err)
	}
	assertLifecycleArtifact(t, hook, nil)
	assertLifecycleArtifact(t, proof, nil)
}
