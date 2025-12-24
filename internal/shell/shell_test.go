package shell

import "testing"

func TestDetectScriptType(t *testing.T) {
	t.Run("detects powershell for multi line script content", func(t *testing.T) {
		powerShellScript := `$ErrorActionPreference = "Stop"
Get-ChildItem -Force
Write-Host "ok"`
		if DetectScriptType(powerShellScript) != ScriptPowerShell {
			t.Fatal("expected PowerShell detection")
		}
	})

	t.Run("detects posix for multi line script content", func(t *testing.T) {
		posixScript := "set -euo pipefail\nif [ -n \"$FOO\" ]; then\n  echo ok\nfi\n"
		if DetectScriptType(posixScript) != ScriptPosix {
			t.Fatal("expected posix detection")
		}
	})

	t.Run("returns unknown for a neutral one liner", func(t *testing.T) {
		neutralCommand := "ctest --test-dir build"
		if DetectScriptType(neutralCommand) != ScriptUnknown {
			t.Fatal("expected unknown for ctest")
		}
	})

	t.Run("returns unknown for ambiguous one liner posix style command", func(t *testing.T) {
		ambiguousCommand := "ls -la"
		if DetectScriptType(ambiguousCommand) != ScriptUnknown {
			t.Fatal("expected unknown for ambiguous one liner")
		}
	})

	t.Run("returns unknown for a single cmdlet name without strong powershell markers", func(t *testing.T) {
		weakPowerShellSignal := "Get-ChildItem"
		if DetectScriptType(weakPowerShellSignal) != ScriptUnknown {
			t.Fatal("expected unknown for weak powershell one liner")
		}
	})

	t.Run("detects powershell for one liner with strong powershell marker", func(t *testing.T) {
		strongPowerShellSignal := "$env:PATH = $env:PATH"
		if DetectScriptType(strongPowerShellSignal) != ScriptPowerShell {
			t.Fatal("expected PowerShell detection for $env marker")
		}
	})

	t.Run("shebang bash forces posix", func(t *testing.T) {
		shebangBash := "#!/usr/bin/env bash\necho ok\n"
		if DetectScriptType(shebangBash) != ScriptPosix {
			t.Fatal("expected posix detection from bash shebang")
		}
	})

	t.Run("shebang pwsh forces powershell", func(t *testing.T) {
		shebangPwsh := "#!/usr/bin/env pwsh\nWrite-Host ok\n"
		if DetectScriptType(shebangPwsh) != ScriptPowerShell {
			t.Fatal("expected powershell detection from pwsh shebang")
		}
	})
}

func TestResolveShell(t *testing.T) {
	t.Run("windows default for unknown is cmd", func(t *testing.T) {
		resolvedShell := Resolve("windows", "", "ctest --test-dir build")
		if resolvedShell != "cmd" {
			t.Fatalf("expected cmd default, got %q", resolvedShell)
		}
	})

	t.Run("linux default for unknown is sh", func(t *testing.T) {
		resolvedShell := Resolve("linux", "", "ctest --test-dir build")
		if resolvedShell != "sh" {
			t.Fatalf("expected sh default, got %q", resolvedShell)
		}
	})

	t.Run("preferred shell wins even when command is unknown", func(t *testing.T) {
		resolvedShell := Resolve("windows", "powershell.exe", "ctest --test-dir build")
		if resolvedShell != "powershell" {
			t.Fatalf("expected powershell, got %q", resolvedShell)
		}
	})

	t.Run("strong powershell one liner resolves to pwsh", func(t *testing.T) {
		resolvedShell := Resolve("windows", "", "$env:FOO = 'bar'")
		if resolvedShell != "pwsh" {
			t.Fatalf("expected pwsh, got %q", resolvedShell)
		}
	})

	t.Run("posix script content resolves to bash", func(t *testing.T) {
		resolvedShell := Resolve("linux", "", "set -euo pipefail\nif [ -n \"$FOO\" ]; then\n  echo ok\nfi\n")
		if resolvedShell != "bash" {
			t.Fatalf("expected bash, got %q", resolvedShell)
		}
	})
}

func TestNormalizeShell(t *testing.T) {
	t.Run("normalizes powershell exe name", func(t *testing.T) {
		if Normalize("powershell.exe") != "powershell" {
			t.Fatal("expected powershell normalization")
		}
	})

	t.Run("normalizes cmd exe name", func(t *testing.T) {
		if Normalize("cmd.exe") != "cmd" {
			t.Fatal("expected cmd normalization")
		}
	})

	t.Run("normalizes bash from windows path", func(t *testing.T) {
		if Normalize("C:\\Tools\\bash.exe") != "bash" {
			t.Fatal("expected bash normalization")
		}
	})
}
