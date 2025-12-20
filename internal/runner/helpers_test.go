package runner

import (
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

func TestLooksLikePowerShell(t *testing.T) {
	ps := `$ErrorActionPreference = "Stop"
Get-ChildItem -Force
Write-Host "ok"`
	if !looksLikePowerShell(ps) {
		t.Fatal("expected PowerShell detection")
	}

	sh := "set -euo pipefail\nif [ -n \"$FOO\" ]; then\n  echo ok\nfi\n"
	if looksLikePowerShell(sh) {
		t.Fatal("did not expect PowerShell detection for posix script")
	}
}
