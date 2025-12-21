package runner

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDirectShellCommandParsesNewlines(t *testing.T) {
	cmd := "bash -lc \"echo one\\necho two\""
	cmd = strings.ReplaceAll(cmd, "\\n", "\n")

	name, args, ok := directShellCommand(cmd)
	if !ok {
		t.Fatalf("expected direct shell command")
	}
	if name != "bash" || len(args) != 2 || args[0] != "-lc" {
		t.Fatalf("unexpected command: %s %v", name, args)
	}
	if !strings.Contains(args[1], "echo one\n") || !strings.Contains(args[1], "echo two") {
		t.Fatalf("expected newline script, got %q", args[1])
	}
}

func TestToShellPath(t *testing.T) {
	msys := toShellPath(`C:\Program Files\Go\bin`, pathFlavorMSYS)
	if msys != "/c/Program Files/Go/bin" {
		t.Fatalf("unexpected msys path: %q", msys)
	}

	wsl := toShellPath(`C:\Program Files\Go\bin`, pathFlavorWSL)
	if wsl != "/mnt/c/Program Files/Go/bin" {
		t.Fatalf("unexpected wsl path: %q", wsl)
	}
}

func TestFixWindowsPathFromPosix(t *testing.T) {
	env := []string{
		"USER=dev",
		"PATH=/c/Program Files/Git/cmd:/c/Windows/System32:/usr/bin",
	}
	fixed := fixWindowsPathFromPosix(env)
	path := ""
	for _, kv := range fixed {
		if strings.HasPrefix(kv, "PATH=") {
			path = strings.TrimPrefix(kv, "PATH=")
			break
		}
	}
	if path == "" {
		t.Fatal("expected PATH to be set")
	}
	if !strings.Contains(path, `C:\Program Files\Git\cmd`) {
		t.Fatalf("expected converted Git path, got %q", path)
	}
	if !strings.Contains(path, `C:\Windows\System32`) {
		t.Fatalf("expected converted System32 path, got %q", path)
	}
	if !strings.Contains(path, "/usr/bin") {
		t.Fatalf("expected posix path preserved, got %q", path)
	}
}

func TestShellCommandDefaultsToCmdOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only behavior")
	}
	name, args := shellCommand("echo ok")
	if !strings.Contains(strings.ToLower(filepath.Base(name)), "cmd") {
		t.Fatalf("expected cmd shell, got %q", name)
	}
	if len(args) < 2 || args[0] != "/C" {
		t.Fatalf("unexpected args: %v", args)
	}
}
