package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func hookCopiedBinaryPath(repo string) string {
	path := filepath.Join(repo, ".git", "hooks", "bin", "build-bouncer")
	if runtime.GOOS == "windows" {
		return path + ".exe"
	}
	return path
}

func TestPathHookUninstallPreservesUntrackedBinaries(t *testing.T) {
	repo := withTempRepo(t)
	binDir := filepath.Join(repo, ".git", "hooks", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"build-bouncer", "build-bouncer.exe", ".build-bouncer-123456.tmp"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
		t.Fatalf("install: %d %s", code, stderr)
	}
	if code, _, stderr := runHookCmd([]string{"uninstall"}); code != exitOK {
		t.Fatalf("uninstall: %d %s", code, stderr)
	}
	for _, name := range []string{"build-bouncer", "build-bouncer.exe", ".build-bouncer-123456.tmp"} {
		data, err := os.ReadFile(filepath.Join(binDir, name))
		if err != nil || string(data) != name {
			t.Fatalf("uninstall changed unrelated file %q: %v", name, err)
		}
	}
}

func TestCopiedHookPreservesOtherPlatformExecutable(t *testing.T) {
	repo := withTempRepo(t)
	binary := hookCopiedBinaryPath(repo)
	other := strings.TrimSuffix(binary, ".exe")
	if runtime.GOOS != "windows" {
		other += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("another platform")
	if err := os.WriteFile(other, data, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runHookCmd([]string{"install"}); code != exitOK {
		t.Fatalf("install: %d %s", code, stderr)
	}
	if _, err := os.Stat(binary + ".sha256"); err != nil {
		t.Fatalf("missing copied executable ownership record: %v", err)
	}
	if code, _, stderr := runHookCmd([]string{"uninstall"}); code != exitOK {
		t.Fatalf("uninstall: %d %s", code, stderr)
	}
	for _, path := range []string{binary, binary + ".sha256"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("owned file %q was not removed: %v", path, err)
		}
	}
	after, err := os.ReadFile(other)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatalf("other-platform executable changed: %v", err)
	}
}

func TestCopiedHookInstallRefusesUntrackedExecutable(t *testing.T) {
	repo := withTempRepo(t)
	binary := hookCopiedBinaryPath(repo)
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("not installed by BuildBouncer")
	if err := os.WriteFile(binary, foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runHookCmd([]string{"install"}); code != exitUsage ||
		!strings.Contains(stderr, "no matching ownership record") {
		t.Fatalf("untracked executable should block installation: %d %q", code, stderr)
	}
	actual, err := os.ReadFile(binary)
	if err != nil || !bytes.Equal(actual, foreign) {
		t.Fatalf("untracked executable was overwritten: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatalf("hook was published despite refused install: %v", err)
	}
	if code, _, stderr := runHookCmd([]string{"install", "--force"}); code != exitOK {
		t.Fatalf("forced install: %d %s", code, stderr)
	}
	if code, _, stderr := runHookCmd([]string{"uninstall"}); code != exitOK {
		t.Fatalf("uninstall of explicitly managed copied executable: %d %s", code, stderr)
	}
}

func TestCopiedHookRefusesChangedOrUntrackedExecutable(t *testing.T) {
	for _, variant := range []string{"modified", "missing record"} {
		t.Run(variant, func(t *testing.T) {
			repo := withTempRepo(t)
			if code, _, stderr := runHookCmd([]string{"install"}); code != exitOK {
				t.Fatalf("install: %d %s", code, stderr)
			}
			binary := hookCopiedBinaryPath(repo)
			hookPath := filepath.Join(repo, ".git", "hooks", "pre-push")
			originalHook, err := os.ReadFile(hookPath)
			if err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "modified":
				if err := os.WriteFile(binary, []byte("user changed executable"), 0o755); err != nil {
					t.Fatal(err)
				}
			case "missing record":
				if err := os.Remove(binary + ".sha256"); err != nil {
					t.Fatal(err)
				}
			}
			for _, action := range [][]string{{"install"}, {"uninstall"}} {
				if code, _, stderr := runHookCmd(action); code != exitUsage ||
					!strings.Contains(stderr, "no matching ownership record") {
					t.Fatalf("%v should refuse unknown binary: %d %q", action, code, stderr)
				}
				hook, err := os.ReadFile(hookPath)
				if err != nil || !bytes.Equal(hook, originalHook) {
					t.Fatalf("%v modified hook: %v", action, err)
				}
			}
			if code, _, stderr := runHookCmd([]string{"uninstall", "--force"}); code != exitOK {
				t.Fatalf("explicit forced cleanup: %d %s", code, stderr)
			}
		})
	}
}

func TestCopiedHookRefusesSymlinkedOwnershipRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlinks may require developer mode")
	}
	repo := withTempRepo(t)
	if code, _, stderr := runHookCmd([]string{"install"}); code != exitOK {
		t.Fatalf("install: %d %s", code, stderr)
	}
	binary := hookCopiedBinaryPath(repo)
	proof := binary + ".sha256"
	if err := os.Remove(proof); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	contents := []byte("unrelated")
	if err := os.WriteFile(foreign, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, proof); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runHookCmd([]string{"uninstall", "--force"}); code != exitUsage ||
		!strings.Contains(stderr, "ownership record is not a regular file") {
		t.Fatalf("symlinked record should reject cleanup: %d %q", code, stderr)
	}
	after, err := os.ReadFile(foreign)
	if err != nil || !bytes.Equal(after, contents) {
		t.Fatalf("symlink target changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "pre-push")); err != nil {
		t.Fatalf("owned hook removed despite unsafe record: %v", err)
	}
}
