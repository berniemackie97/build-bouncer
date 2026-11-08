package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/hooks"
)

func TestHookStatusReportsNonExecutableHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX execute permissions are not used by Windows Git")
	}
	root := withTempRepo(t)
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
		t.Fatalf("install: %s", stderr)
	}
	hookPath := filepath.Join(root, ".git", "hooks", "pre-push")
	if err := os.Chmod(hookPath, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := hooks.GetStatus()
	if err != nil || !st.Installed || !st.Ours || st.Executable {
		t.Fatalf("incorrect inactive-hook status: %+v, %v", st, err)
	}
	if code, stdout, stderr := runHookCmd([]string{"status"}); code != exitOK ||
		!strings.Contains(stdout, "present but not executable by Git") ||
		!strings.Contains(stdout, "executable by Git: false") {
		t.Fatalf("inactive hook misreported: %d %q %q", code, stdout, stderr)
	}
}

func TestHookRefusesForeignMarkerOutsideHeader(t *testing.T) {
	root := withTempRepo(t)
	hookPath := filepath.Join(root, ".git", "hooks", "pre-push")
	original := []byte("#!/bin/sh\necho foreign\n# build-bouncer pre-push hook v3\n")
	if err := os.WriteFile(hookPath, original, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := hooks.GetStatus()
	if err != nil || !st.Installed || st.Ours || !st.Executable {
		t.Fatalf("foreign hook was misidentified: %+v, %v", st, err)
	}
	if code, stdout, stderr := runHookCmd([]string{"status"}); code != exitOK ||
		!strings.Contains(stdout, "foreign hook present") {
		t.Fatalf("foreign hook status missing: %d %q %q", code, stdout, stderr)
	}
	for _, action := range [][]string{{"install", "--no-copy"}, {"uninstall"}} {
		if code, _, stderr := runHookCmd(action); code != exitUsage ||
			!strings.Contains(stderr, "not installed by build-bouncer") {
			t.Fatalf("%v unexpectedly accepted foreign hook: %d %q", action, code, stderr)
		}
		data, err := os.ReadFile(hookPath)
		if err != nil || !bytes.Equal(original, data) {
			t.Fatalf("%v changed foreign hook: %v", action, err)
		}
	}
}

func TestHookStatusDoesNotFollowSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlinks may require developer mode")
	}
	root := withTempRepo(t)
	foreign := filepath.Join(t.TempDir(), "foreign.sh")
	original := []byte("#!/bin/sh\n# build-bouncer pre-push hook v3\nexit 0\n")
	if err := os.WriteFile(foreign, original, 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(root, ".git", "hooks", "pre-push")
	if err := os.Symlink(foreign, hookPath); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runHookCmd([]string{"status"}); code != exitUsage ||
		!strings.Contains(stderr, "not a regular file") {
		t.Fatalf("symlink status not rejected: %d %q", code, stderr)
	}
	if code, _, stderr := runHookCmd([]string{"install", "--force", "--no-copy"}); code != exitUsage ||
		!strings.Contains(stderr, "not a regular file") {
		t.Fatalf("symlink install not rejected: %d %q", code, stderr)
	}
	data, err := os.ReadFile(foreign)
	if err != nil || !bytes.Equal(original, data) {
		t.Fatalf("symlink target changed: %v", err)
	}
}

func TestHookStatusCopiedBinaryMatchesCurrentOSAndFileType(t *testing.T) {
	root := withTempRepo(t)
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
		t.Fatalf("install: %s", stderr)
	}
	binDir := filepath.Join(root, ".git", "hooks", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	unixPath := filepath.Join(binDir, "build-bouncer")
	windowsPath := unixPath + ".exe"
	host, foreign := unixPath, windowsPath
	if runtime.GOOS == "windows" {
		host, foreign = windowsPath, unixPath
	}
	if err := os.WriteFile(foreign, []byte("foreign platform"), 0o755); err != nil {
		t.Fatal(err)
	}
	assertCopy := func(want bool) {
		t.Helper()
		st, err := hooks.GetStatus()
		if err != nil || st.CopiedBinary != want {
			t.Fatalf("current-OS executable availability = %v, want %v (error %v)", st.CopiedBinary, want, err)
		}
	}
	assertCopy(false)
	if err := os.WriteFile(host, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		assertCopy(true)
	} else {
		assertCopy(false)
		if err := os.Chmod(host, 0o755); err != nil {
			t.Fatal(err)
		}
		assertCopy(true)
	}
	if err := os.Remove(host); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := os.Mkdir(host, 0o755); err != nil {
			t.Fatal(err)
		}
	} else if err := os.Symlink(foreign, host); err != nil {
		t.Fatal(err)
	}
	assertCopy(false)
}

func TestHookStatusDetectsModifiedAndLegacyInstallation(t *testing.T) {
	root := withTempRepo(t)
	if code, _, stderr := runHookCmd([]string{"install", "--no-copy"}); code != exitOK {
		t.Fatalf("install: %s", stderr)
	}
	hookPath := filepath.Join(root, ".git", "hooks", "pre-push")
	original, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	st, err := hooks.GetStatus()
	if err != nil || !st.Ours || !st.Current {
		t.Fatalf("current hook misidentified: %+v, %v", st, err)
	}
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{name: "modified current hook", body: append(append([]byte(nil), original...), []byte("# changed\n")...)},
		{name: "legacy hook", body: []byte("#!/bin/sh\n# build-bouncer pre-push hook v1\necho previous\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(hookPath, tc.body, 0o755); err != nil {
				t.Fatal(err)
			}
			st, err := hooks.GetStatus()
			if err != nil || !st.Ours || st.Current {
				t.Fatalf("outdated hook incorrectly reported current: %+v, %v", st, err)
			}
			if code, out, stderr := runHookCmd([]string{"status"}); code != exitOK ||
				!strings.Contains(out, "outdated or modified") ||
				!strings.Contains(out, "current build-bouncer hook: false") {
				t.Fatalf("missing outdated-hook diagnostic: %d %q %q", code, out, stderr)
			}
			for _, action := range [][]string{{"install", "--no-copy"}, {"uninstall"}} {
				if code, _, stderr := runHookCmd(action); code != exitUsage ||
					!strings.Contains(stderr, "modified or older") {
					t.Fatalf("%v should refuse modified hook: %d %q", action, code, stderr)
				}
				if actual, err := os.ReadFile(hookPath); err != nil || !bytes.Equal(actual, tc.body) {
					t.Fatalf("%v modified an existing hook: %v", action, err)
				}
			}
		})
	}
	if code, _, stderr := runHookCmd([]string{"install", "--force", "--no-copy"}); code != exitOK {
		t.Fatalf("explicit reinstall of older hook: %s", stderr)
	}
	st, err = hooks.GetStatus()
	if err != nil || !st.Ours || !st.Current {
		t.Fatalf("reinstall did not restore current hook: %+v, %v", st, err)
	}
}
