package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
