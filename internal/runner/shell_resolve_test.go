package runner

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestShellCommand(t *testing.T) {
	name, args := shellCommand("echo hi")

	if runtime.GOOS == "windows" {
		if name != "cmd.exe" || !slices.Equal(args, []string{"/C", "echo hi"}) {
			t.Fatalf("shellCommand() = (%q, %v), want (%q, %v)", name, args, "cmd.exe", []string{"/C", "echo hi"})
		}
		return
	}

	if name != "sh" || !slices.Equal(args, []string{"-c", "echo hi"}) {
		t.Fatalf("shellCommand() = (%q, %v), want (%q, %v)", name, args, "sh", []string{"-c", "echo hi"})
	}
}

func TestSplitExecutableAndArgs(t *testing.T) {
	tests := []struct {
		name     string
		spec     string
		wantExe  string
		wantArgs []string
	}{
		{name: "empty", spec: "", wantExe: "", wantArgs: nil},
		{name: "single word", spec: "pwsh", wantExe: "pwsh", wantArgs: nil},
		{
			name:     "quoted path with args",
			spec:     `"C:\Program Files\PowerShell\7\pwsh.exe" -NoProfile -NoLogo`,
			wantExe:  `C:\Program Files\PowerShell\7\pwsh.exe`,
			wantArgs: []string{"-NoProfile", "-NoLogo"},
		},
		{
			name:     "quoted path no args",
			spec:     `"C:\Path With Space\sh.exe"`,
			wantExe:  `C:\Path With Space\sh.exe`,
			wantArgs: nil,
		},
		{
			name:     "unterminated quote is treated as exe",
			spec:     `"C:\Nope`,
			wantExe:  `"C:\Nope`,
			wantArgs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExe, gotArgs := splitExecutableAndArgs(tt.spec)
			if gotExe != tt.wantExe || !slices.Equal(gotArgs, tt.wantArgs) {
				t.Fatalf("splitExecutableAndArgs(%q) = (%q, %v), want (%q, %v)", tt.spec, gotExe, gotArgs, tt.wantExe, tt.wantArgs)
			}
		})
	}
}

func TestCutFirstToken(t *testing.T) {
	t.Run("unquoted", func(t *testing.T) {
		tok, rest := cutFirstToken("foo bar baz")
		if tok != "foo" || rest != "bar baz" {
			t.Fatalf("cutFirstToken() = (%q, %q), want (%q, %q)", tok, rest, "foo", "bar baz")
		}
	})

	t.Run("quoted path", func(t *testing.T) {
		in := `"C:\Program Files\Git\bin\bash.exe" -lc "echo hi"`
		tok, rest := cutFirstToken(in)
		if tok != `C:\Program Files\Git\bin\bash.exe` || rest != `-lc "echo hi"` {
			t.Fatalf("cutFirstToken() = (%q, %q), want (%q, %q)", tok, rest, `C:\Program Files\Git\bin\bash.exe`, `-lc "echo hi"`)
		}
	})

	t.Run("unterminated quoted token returns empty", func(t *testing.T) {
		tok, rest := cutFirstToken(`"C:\Nope`)
		if tok != "" || rest != "" {
			t.Fatalf("cutFirstToken() = (%q, %q), want empty", tok, rest)
		}
	})
}

func TestUnquoteShellArg(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{name: "double quoted", in: `"echo hi"`, want: "echo hi", wantOK: true},
		{name: "single quoted", in: `'echo hi'`, want: "echo hi", wantOK: true},
		{name: "double quoted with escapes", in: `"a\"b"`, want: `a"b`, wantOK: true},
		{name: "not quoted", in: `echo hi`, want: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := unquoteShellArg(tt.in)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("unquoteShellArg(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestParseDirectShellInvocation(t *testing.T) {
	t.Run("bash -lc with quoted script", func(t *testing.T) {
		name, args, ok := parseDirectShellInvocation(`bash -lc "make test"`, "bash")
		if !ok {
			t.Fatal("expected ok")
		}
		if name != "bash" || !slices.Equal(args, []string{"-lc", "make test"}) {
			t.Fatalf("got (%q, %v), want (%q, %v)", name, args, "bash", []string{"-lc", "make test"})
		}
	})

	t.Run("accepts full path ending in bash", func(t *testing.T) {
		name, args, ok := parseDirectShellInvocation(`/usr/bin/bash -c "echo hi"`, "bash")
		if !ok {
			t.Fatal("expected ok")
		}
		if name != "bash" || !slices.Equal(args, []string{"-c", "echo hi"}) {
			t.Fatalf("got (%q, %v), want (%q, %v)", name, args, "bash", []string{"-c", "echo hi"})
		}
	})

	t.Run("rejects unsupported flag", func(t *testing.T) {
		_, _, ok := parseDirectShellInvocation(`bash -x "echo hi"`, "bash")
		if ok {
			t.Fatal("expected not ok")
		}
	})

	t.Run("rejects unquoted script", func(t *testing.T) {
		_, _, ok := parseDirectShellInvocation(`bash -lc echo`, "bash")
		if ok {
			t.Fatal("expected not ok")
		}
	})
}

func TestCommandForShell(t *testing.T) {
	t.Run("pwsh builds standard args", func(t *testing.T) {
		name, args, ok := commandForShell("pwsh", "Get-ChildItem")
		if !ok {
			t.Fatal("expected ok")
		}

		wantName := "pwsh"
		wantArgs := []string{"-NoProfile", "-NonInteractive", "-Command", "Get-ChildItem"}

		if name != wantName || !slices.Equal(args, wantArgs) {
			t.Fatalf("got (%q, %v), want (%q, %v)", name, args, wantName, wantArgs)
		}
	})

	t.Run("bash path avoids preferWindowsShell lookups", func(t *testing.T) {
		name, args, ok := commandForShell("/usr/bin/bash", "make test")
		if !ok {
			t.Fatal("expected ok")
		}
		wantName := "/usr/bin/bash"
		wantArgs := []string{"-lc", "make test"}

		if name != wantName || !slices.Equal(args, wantArgs) {
			t.Fatalf("got (%q, %v), want (%q, %v)", name, args, wantName, wantArgs)
		}
	})

	t.Run("cmd always returns cmd.exe and puts prefix args before /C", func(t *testing.T) {
		name, args, ok := commandForShell(`cmd /D`, "dir")
		if !ok {
			t.Fatal("expected ok")
		}
		wantName := "cmd.exe"
		wantArgs := []string{"/D", "/C", "dir"}

		if name != wantName || !slices.Equal(args, wantArgs) {
			t.Fatalf("got (%q, %v), want (%q, %v)", name, args, wantName, wantArgs)
		}
	})

	t.Run("unknown shell treats command as final arg", func(t *testing.T) {
		name, args, ok := commandForShell(`weirdshell -x -y`, "do thing")
		if !ok {
			t.Fatal("expected ok")
		}
		wantName := "weirdshell"
		wantArgs := []string{"-x", "-y", "do thing"}

		if name != wantName || !slices.Equal(args, wantArgs) {
			t.Fatalf("got (%q, %v), want (%q, %v)", name, args, wantName, wantArgs)
		}
	})
}

func TestResolveCommand_Order(t *testing.T) {
	t.Run("explicit shell wins", func(t *testing.T) {
		name, args := resolveCommand("pwsh", "Write-Host ok", "")
		if name != "pwsh" {
			t.Fatalf("expected pwsh, got %q", name)
		}
		if len(args) < 4 || args[len(args)-1] != "Write-Host ok" {
			t.Fatalf("unexpected args: %v", args)
		}
	})

	t.Run("direct shell invocation is recognized when shell is empty", func(t *testing.T) {
		name, args := resolveCommand("", `bash -lc "echo hi"`, "")
		base := strings.ToLower(filepath.Base(name))
		if base != "bash" && base != "bash.exe" {
			t.Fatalf("expected bash executable, got %q", name)
		}
		if !slices.Equal(args, []string{"-lc", "echo hi"}) {
			t.Fatalf("unexpected args: %v", args)
		}
	})

	t.Run("fallback shell is used when command is not a direct invocation", func(t *testing.T) {
		name, args := resolveCommand("", "echo hi", "cmd")
		if name != "cmd.exe" {
			t.Fatalf("expected cmd.exe, got %q", name)
		}
		if !slices.Equal(args, []string{"/C", "echo hi"}) {
			t.Fatalf("unexpected args: %v", args)
		}
	})

	t.Run("OS default fallback when nothing else applies", func(t *testing.T) {
		name, args := resolveCommand("", "echo hi", "")
		if runtime.GOOS == "windows" {
			if name != "cmd.exe" || !slices.Equal(args, []string{"/C", "echo hi"}) {
				t.Fatalf("got (%q, %v), want (cmd.exe, [/C echo hi])", name, args)
			}
			return
		}
		if name != "sh" || !slices.Equal(args, []string{"-c", "echo hi"}) {
			t.Fatalf("got (%q, %v), want (sh, [-c echo hi])", name, args)
		}
	})
}
