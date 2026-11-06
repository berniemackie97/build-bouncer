package runner

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
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
		if name != "/usr/bin/bash" || !slices.Equal(args, []string{"-c", "echo hi"}) {
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
	tests := []struct {
		name       string
		spec       string
		command    string
		executable string
		args       []string
	}{
		{name: "powershell", spec: "pwsh", command: "Get-ChildItem", executable: "pwsh",
			args: []string{"-NoProfile", "-NonInteractive", "-Command", "Get-ChildItem"}},
		{name: "POSIX path", spec: "/usr/bin/bash", command: "make test", executable: "/usr/bin/bash",
			args: []string{"-lc", "make test"}},
		{name: "Windows path with spaces", spec: "C:\\Program Files\\Git\\bin\\bash.exe", command: "make test",
			executable: "C:\\Program Files\\Git\\bin\\bash.exe", args: []string{"-lc", "make test"}},
		{name: "quoted Windows path", spec: "\"C:\\Program Files\\PowerShell\\7\\pwsh.exe\"",
			command: "Write-Host ok", executable: "C:\\Program Files\\PowerShell\\7\\pwsh.exe",
			args: []string{"-NoProfile", "-NonInteractive", "-Command", "Write-Host ok"}},
		{name: "custom executable", spec: "custom-shell", command: "run",
			executable: "custom-shell", args: []string{"run"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, args, err := commandForShell(tc.spec, tc.command)
			if err != nil || name != tc.executable || !slices.Equal(args, tc.args) {
				t.Fatalf("commandForShell(%q) = %q, %q, %v; want %q, %q", tc.spec, name, args, err, tc.executable, tc.args)
			}
		})
	}
	for _, spec := range []string{"bash -lc", "cmd /D", "\"C:\\Program Files\\Git\\bin\\bash.exe\" -lc", "\"C:\\Unterminated"} {
		if _, _, err := commandForShell(spec, "echo test"); err == nil {
			t.Fatalf("accepted shell arguments or malformed spec %q", spec)
		}
	}
}

func TestResolveCommand_Order(t *testing.T) {
	t.Run("explicit shell wins", func(t *testing.T) {
		name, args, err := resolveCommand("pwsh", "Write-Host ok", "")
		if err != nil {
			t.Fatal(err)
		}
		if name != "pwsh" {
			t.Fatalf("expected pwsh, got %q", name)
		}
		if len(args) < 4 || args[len(args)-1] != "Write-Host ok" {
			t.Fatalf("unexpected args: %v", args)
		}
	})

	t.Run("direct shell invocation is recognized when shell is empty", func(t *testing.T) {
		name, args, err := resolveCommand("", `bash -lc "echo hi"`, "")
		if err != nil {
			t.Fatal(err)
		}
		base := strings.ToLower(filepath.Base(name))
		if base != "bash" && base != "bash.exe" {
			t.Fatalf("expected bash executable, got %q", name)
		}
		if !slices.Equal(args, []string{"-lc", "echo hi"}) {
			t.Fatalf("unexpected args: %v", args)
		}
	})

	t.Run("fallback shell is used when command is not a direct invocation", func(t *testing.T) {
		name, args, err := resolveCommand("", "echo hi", "cmd")
		if err != nil {
			t.Fatal(err)
		}
		if name != "cmd.exe" {
			t.Fatalf("expected cmd.exe, got %q", name)
		}
		if !slices.Equal(args, []string{"/C", "echo hi"}) {
			t.Fatalf("unexpected args: %v", args)
		}
	})

	t.Run("OS default fallback when nothing else applies", func(t *testing.T) {
		name, args, err := resolveCommand("", "echo hi", "")
		if runtime.GOOS == "windows" {
			if err != nil {
				t.Fatal(err)
			}
			if name != "cmd.exe" || !slices.Equal(args, []string{"/C", "echo hi"}) {
				t.Fatalf("got (%q, %v), want (cmd.exe, [/C echo hi])", name, args)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if name != "sh" || !slices.Equal(args, []string{"-c", "echo hi"}) {
			t.Fatalf("got (%q, %v), want (sh, [-c echo hi])", name, args)
		}
	})
}

func TestDirectPowerShellInvocationPreservesExecutablePath(t *testing.T) {
	spec := "\"C:\\Program Files\\PowerShell\\7\\pwsh.exe\" -NoProfile -Command \"Write-Host ok\""
	name, args, ok := parseDirectPowerShellInvocation(spec, "pwsh")
	if !ok || name != "C:\\Program Files\\PowerShell\\7\\pwsh.exe" ||
		!slices.Equal(args, []string{"-NoProfile", "-Command", "Write-Host ok"}) {
		t.Fatalf("direct invocation changed explicit path: %q, %q, %t", name, args, ok)
	}
}

func TestInvalidExplicitShellDoesNotFallBack(t *testing.T) {
	if _, _, err := resolveCommand("bash -lc", "echo ok", "sh"); err == nil {
		t.Fatal("invalid explicit shell must not fall back to sh")
	}
}

func TestConfiguredShellPathWithSpacesExecutes(t *testing.T) {
	var source, targetName, script string
	if runtime.GOOS == "windows" {
		source = os.Getenv("ComSpec")
		targetName = "cmd.exe"
		script = "echo working"
	} else {
		var err error
		source, err = exec.LookPath("sh")
		if err != nil {
			t.Fatal(err)
		}
		targetName = "sh"
		script = "printf working"
	}
	directory := filepath.Join(t.TempDir(), "shell binaries with spaces")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, targetName)
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	executable, args, err := resolveCommand("\""+target+"\"", script, "")
	if err != nil || executable != target {
		t.Fatalf("shell executable changed: %q != %q: %v", executable, target, err)
	}
	result, err := exec.Command(executable, args...).CombinedOutput()
	if err != nil || strings.TrimSpace(string(result)) != "working" {
		t.Fatalf("execute shell with spaces: %s %v", result, err)
	}

	cfg := &config.Config{Checks: []config.Check{{Name: "path with spaces", Run: script, Shell: "\"" + target + "\""}}}
	report, err := RunAllReport(t.TempDir(), cfg, Options{})
	if err != nil || len(report.Failures) != 0 || len(report.Unavailable) != 0 || len(report.Skipped) != 0 {
		t.Fatalf("runner failed to execute configured shell: %+v, %v", report, err)
	}
}

func TestExplicitShellPathIsNeverReplaced(t *testing.T) {
	for _, path := range []string{"/usr/bin/bash", `C:\Windows\System32\bash.exe`, `C:\Program Files\Git\bin\bash.exe`} {
		selected, err := preferWindowsShell(path)
		if err != nil || selected != path {
			t.Fatalf("explicit shell path %q was replaced with %q: %v", path, selected, err)
		}
	}
}

func TestExplicitWSLShellArguments(t *testing.T) {
	name, args, err := commandForShell("wsl.exe", "printf '%s' 'two words'")
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Fatal("WSL must not be selected on a non-Windows host")
		}
		return
	}
	if err != nil || name != "wsl.exe" ||
		!slices.Equal(args, []string{"--exec", "sh", "-c", "printf '%s' 'two words'"}) {
		t.Fatalf("unexpected WSL command: %q %q, %v", name, args, err)
	}
}

func TestRelativeShellPathResolvesFromRepoRoot(t *testing.T) {
	var source, name, script string
	if runtime.GOOS == "windows" {
		source = os.Getenv("ComSpec")
		name, script = "cmd.exe", "echo working"
	} else {
		var err error
		source, err = exec.LookPath("sh")
		if err != nil {
			t.Fatal(err)
		}
		name, script = "sh", "printf working"
	}
	root := t.TempDir()
	relative := filepath.Join("bin with spaces", name)
	absolute := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(absolute, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Checks: []config.Check{{
		Name: "relative shell", Shell: "\"" + relative + "\"", Cwd: "work", Run: script,
	}}}
	if executable, err := ResolveCheckExecutableAt(root, cfg.Checks[0].Shell, script, nil); err != nil ||
		!strings.EqualFold(executable, absolute) {
		t.Fatalf("root-relative shell resolved to %q instead of %q: %v", executable, absolute, err)
	}
	report, err := RunAllReport(root, cfg, Options{})
	if err != nil || len(report.Failures) != 0 || len(report.Unavailable) != 0 || len(report.Skipped) != 0 {
		t.Fatalf("root-relative shell did not execute: %+v, %v", report, err)
	}
}

func TestUnavailableShellHasNoFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-shell")
	if _, err := ResolveCheckExecutable(path, "echo ok"); err == nil ||
		!strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected missing shell error, got %v", err)
	}
}
