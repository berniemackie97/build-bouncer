package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/hooks"
)

func TestCopiedHookStatusRequiresMatchingExecutableRecord(t *testing.T) {
	for _, variant := range []string{"current", "modified", "missing record", "missing binary", "not executable"} {
		t.Run(variant, func(t *testing.T) {
			if variant == "not executable" && runtime.GOOS == "windows" {
				t.Skip("Windows does not use POSIX execute permissions")
			}
			repo := withTempRepo(t)
			if code, _, stderr := runHookCmd([]string{"install"}); code != exitOK {
				t.Fatalf("install copied hook: %d %s", code, stderr)
			}
			binary := hookCopiedBinaryPath(repo)
			switch variant {
			case "modified":
				if err := os.WriteFile(binary, []byte("modified executable"), 0o755); err != nil {
					t.Fatal(err)
				}
			case "missing record":
				if err := os.Remove(binary + ".sha256"); err != nil {
					t.Fatal(err)
				}
			case "missing binary":
				if err := os.Remove(binary); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(binary + ".sha256"); err != nil {
					t.Fatal(err)
				}
			case "not executable":
				if err := os.Chmod(binary, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			st, err := hooks.GetStatus()
			if err != nil {
				t.Fatal(err)
			}
			ready := variant == "current"
			if !st.Installed || !st.Ours || !st.Current || !st.UsesCopiedBinary {
				t.Fatalf("generated copied hook misidentified: %+v", st)
			}
			if st.CopiedBinaryMatchesRecord != (variant == "current" || variant == "not executable") {
				t.Fatalf("unexpected copied binary digest status: %+v", st)
			}
			if st.CopiedBinary != (variant != "missing binary" && variant != "not executable") {
				t.Fatalf("unexpected copied binary executable status: %+v", st)
			}
			code, stdout, stderr := runHookCmd([]string{"status"})
			if code != exitOK {
				t.Fatalf("hook status: %d %s", code, stderr)
			}
			if ready {
				if !strings.Contains(stdout, "pre-push hook: installed") ||
					!strings.Contains(stdout, "copied binary matches ownership record: true") {
					t.Fatalf("current copied hook not recognized: %q", stdout)
				}
			} else if !strings.Contains(stdout, "copied executable missing or unverified") ||
				strings.Contains(stdout, "pre-push hook: installed") {
				t.Fatalf("unverified copied hook reported installed: %q", stdout)
			}
		})
	}
}

func TestCopiedHookStatusRejectsInvalidOwnershipRecord(t *testing.T) {
	repo := withTempRepo(t)
	if code, _, stderr := runHookCmd([]string{"install"}); code != exitOK {
		t.Fatalf("install copied hook: %d %s", code, stderr)
	}
	binary := hookCopiedBinaryPath(repo)
	if err := os.WriteFile(binary+".sha256", []byte("invalid hash"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := runHookCmd([]string{"status"}); code != exitUsage ||
		!strings.Contains(stderr, "invalid copied executable ownership record") ||
		strings.Contains(stdout, "pre-push hook: installed") {
		t.Fatalf("invalid ownership record misreported: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "pre-push")); err != nil {
		t.Fatalf("status must not modify the hook: %v", err)
	}
}

func TestPathHookStatusDoesNotDependOnCopiedBinaryOwnership(t *testing.T) {
	repo := withTempRepo(t)
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
		t.Fatalf("install PATH hook: %d %s", code, stderr)
	}
	binary := hookCopiedBinaryPath(repo)
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("user-owned executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary+".sha256", []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := hooks.GetStatus()
	if err != nil || !st.Installed || !st.Current || st.UsesCopiedBinary {
		t.Fatalf("PATH-only hook depends on copied binary state: %+v %v", st, err)
	}
	if code, stdout, stderr := runHookCmd([]string{"status"}); code != exitOK ||
		!strings.Contains(stdout, "pre-push hook: installed") ||
		strings.Contains(stdout, "copied binary matches ownership record:") {
		t.Fatalf("incorrect PATH-only status: %d %q %q", code, stdout, stderr)
	}
}

func TestHookStatusExplainsOrphanAndLifecycleLock(t *testing.T) {
	repo := withTempRepo(t)
	if code, _, stderr := runHookCmd([]string{"install"}); code != exitOK {
		t.Fatalf("install: %d %s", code, stderr)
	}
	binary := hookCopiedBinaryPath(repo)
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runHookCmd([]string{"status"})
	if code != exitOK || !strings.Contains(stdout, "orphaned ownership record: inspect before") ||
		strings.Contains(stdout, "pre-push hook: installed") {
		t.Fatalf("orphan status incorrectly reported: %d %q %q", code, stdout, stderr)
	}
	lockPath := filepath.Join(repo, ".git", "hooks", ".build-bouncer-hook.lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, action := range [][]string{{"status"}, {"install", "--force"}, {"uninstall", "--force"}} {
		code, _, stderr := runHookCmd(action)
		if code != exitUsage || !strings.Contains(stderr, "hook lifecycle lock") {
			t.Fatalf("%v bypassed active lifecycle lock: %d %q", action, code, stderr)
		}
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runHookCmd([]string{"install", "--force"}); code != exitOK {
		t.Fatalf("explicit orphan repair: %d %s", code, stderr)
	}
	if code, stdout, stderr := runHookCmd([]string{"status"}); code != exitOK ||
		!strings.Contains(stdout, "pre-push hook: installed") ||
		strings.Contains(stdout, "orphaned ownership record") {
		t.Fatalf("repaired hook status incorrect: %d %q %q", code, stdout, stderr)
	}
}
