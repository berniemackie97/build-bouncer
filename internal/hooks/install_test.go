package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHookSelectsCopiedBinaryForHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell selection is exercised on Linux and macOS")
	}

	cases := []struct {
		name    string
		host    string
		copy    bool
		native  bool
		windows bool
		want    string
	}{
		{name: "macOS ignores stale Windows executable", host: "Darwin", copy: true, windows: true, want: "build-bouncer"},
		{name: "Linux prefers native executable", host: "Linux", copy: true, native: true, windows: true, want: "native"},
		{name: "Git Bash prefers Windows executable", host: "MINGW64_NT-10.0", copy: true, native: true, windows: true, want: "windows"},
		{name: "Git Bash ignores native executable", host: "MSYS_NT-10.0", copy: true, native: true, want: "build-bouncer"},
		{name: "PATH installation ignores copied binaries", host: "Darwin", native: true, windows: true, want: "build-bouncer"},
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
			for name, data := range map[string]string{
				"uname":         "#!/bin/sh\nprintf '%s\\n' '" + tc.host + "'\n",
				"build-bouncer": "#!/bin/sh\nexit 0\n",
			} {
				if err := os.WriteFile(filepath.Join(shimDir, name), []byte(data), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			nativePath := filepath.Join(binDir, "build-bouncer")
			windowsPath := nativePath + ".exe"
			if tc.native {
				if err := os.WriteFile(nativePath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if tc.windows {
				if err := os.WriteFile(windowsPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			body := renderPrePushHook(tc.copy)
			end := strings.Index(body, "if [ -t 0 ]; then")
			if end < 0 {
				t.Fatal("hook command dispatch was not found")
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
			want := tc.want
			switch want {
			case "native":
				want = nativePath
			case "windows":
				want = windowsPath
			}
			if got := strings.TrimSpace(string(output)); got != want {
				t.Fatalf("selected %q, want %q", got, want)
			}
		})
	}
}
