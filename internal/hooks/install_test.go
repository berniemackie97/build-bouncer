package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHookSelectsCopiedBinaryForHostAndArchitecture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell selection is exercised on Linux and macOS")
	}
	cases := []struct {
		name, system, machine    string
		copy, hasCopy            bool
		binarySystem, binaryArch string
	}{
		{name: "Darwin ARM64 selects host binary", system: "Darwin", machine: "arm64", copy: true, hasCopy: true, binarySystem: "darwin", binaryArch: "arm64"},
		{name: "Darwin AMD64 selects host binary", system: "Darwin", machine: "x86_64", copy: true, hasCopy: true, binarySystem: "darwin", binaryArch: "amd64"},
		{name: "Linux AMD64 selects host binary", system: "Linux", machine: "x86_64", copy: true, hasCopy: true, binarySystem: "linux", binaryArch: "amd64"},
		{name: "Linux ARM64 selects host binary", system: "Linux", machine: "aarch64", copy: true, hasCopy: true, binarySystem: "linux", binaryArch: "arm64"},
		{name: "Git Bash AMD64 selects Windows binary", system: "MINGW64_NT-10.0", machine: "x86_64", copy: true, hasCopy: true, binarySystem: "windows", binaryArch: "amd64"},
		{name: "Git Bash ARM64 selects Windows binary", system: "MSYS_NT-10.0", machine: "aarch64", copy: true, hasCopy: true, binarySystem: "windows", binaryArch: "arm64"},
		{name: "Unknown machine falls back to PATH", system: "Darwin", machine: "unknown", copy: true, hasCopy: true, binarySystem: "darwin", binaryArch: "arm64"},
		{name: "Unknown system falls back to PATH", system: "FreeBSD", machine: "x86_64", copy: true, hasCopy: true, binarySystem: "darwin", binaryArch: "amd64"},
		{name: "PATH mode never picks copy", system: "Darwin", machine: "arm64", copy: false, hasCopy: true, binarySystem: "darwin", binaryArch: "arm64"},
		{name: "Linux ignores Darwin copy", system: "Linux", machine: "arm64", copy: true, hasCopy: true, binarySystem: "darwin", binaryArch: "arm64"},
		{name: "Git Bash ignores Linux copy", system: "CYGWIN_NT", machine: "x86_64", copy: true, hasCopy: true, binarySystem: "linux", binaryArch: "amd64"},
		{name: "Darwin ignores Linux copy", system: "Darwin", machine: "arm64", copy: true, hasCopy: true, binarySystem: "linux", binaryArch: "arm64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			binDir := filepath.Join(root, "hooks", "bin")
			shimDir := filepath.Join(root, "path")
			for _, dir := range []string{binDir, shimDir} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			uname := "#!/bin/sh\ncase \"$1\" in\n  -s) printf '%s\\n' '" + tc.system + "' ;;\n  -m) printf '%s\\n' '" + tc.machine + "' ;;\nesac\n"
			for name, body := range map[string]string{
				"uname":         uname,
				"build-bouncer": "#!/bin/sh\nexit 0\n",
			} {
				if err := os.WriteFile(filepath.Join(shimDir, name), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			binary := filepath.Join(binDir, "build-bouncer-"+tc.binarySystem+"-"+tc.binaryArch)
			if tc.hasCopy {
				if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			body := renderPrePushHook(tc.copy)
			end := strings.Index(body, "if [ -t 0 ]; then")
			if end < 0 {
				t.Fatal("hook check dispatch missing")
			}
			body = body[:end] + "printf '%s\\n' \"$bb\"\n"
			hookPath := filepath.Join(root, "hooks", "pre-push")
			if err := os.WriteFile(hookPath, []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", hookPath)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run hook resolver: %v: %s", err, output)
			}
			want := "build-bouncer"
			matchesSystem := tc.system == "Darwin" && tc.binarySystem == "darwin" ||
				tc.system == "Linux" && tc.binarySystem == "linux" ||
				(strings.HasPrefix(tc.system, "MINGW") || strings.HasPrefix(tc.system, "MSYS") || strings.HasPrefix(tc.system, "CYGWIN")) && tc.binarySystem == "windows"
			matchesArch := tc.machine == "arm64" && tc.binaryArch == "arm64" ||
				tc.machine == "aarch64" && tc.binaryArch == "arm64" ||
				tc.machine == "x86_64" && tc.binaryArch == "amd64"
			if tc.copy && tc.hasCopy && matchesSystem && matchesArch {
				want = binary
			}
			if got := strings.TrimSpace(string(output)); got != want {
				t.Fatalf("selected %q, want %q", got, want)
			}
		})
	}
}
