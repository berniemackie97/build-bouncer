package shell

import "testing"

func TestDetectScriptType(t *testing.T) {
	ps := `$ErrorActionPreference = "Stop"
Get-ChildItem -Force
Write-Host "ok"`
	if DetectScriptType(ps) != ScriptPowerShell {
		t.Fatal("expected PowerShell detection")
	}

	sh := "set -euo pipefail\nif [ -n \"$FOO\" ]; then\n  echo ok\nfi\n"
	if DetectScriptType(sh) != ScriptPosix {
		t.Fatal("expected posix detection")
	}

	if DetectScriptType("ctest --test-dir build") != ScriptUnknown {
		t.Fatal("expected unknown for ctest")
	}
}

func TestResolveShell(t *testing.T) {
	cmd := Resolve("windows", "", "ctest --test-dir build")
	if cmd != "cmd" {
		t.Fatalf("expected cmd default, got %q", cmd)
	}

	ps := Resolve("windows", "", "Get-ChildItem")
	if ps != "pwsh" {
		t.Fatalf("expected pwsh, got %q", ps)
	}

	posix := Resolve("linux", "", "ls -la")
	if posix != "bash" {
		t.Fatalf("expected bash, got %q", posix)
	}
}

func TestNormalizeShell(t *testing.T) {
	if Normalize("powershell.exe") != "powershell" {
		t.Fatal("expected powershell normalization")
	}
	if Normalize("cmd.exe") != "cmd" {
		t.Fatal("expected cmd normalization")
	}
	if Normalize("C:\\Tools\\bash.exe") != "bash" {
		t.Fatal("expected bash normalization")
	}
}
