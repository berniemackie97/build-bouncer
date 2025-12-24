package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindEnvVar(t *testing.T) {
	env := []string{
		"FOO=bar",
		"Path=/usr/bin",
		"BAZ=qux",
	}

	idx, val := findEnvVar(env, "PATH")
	if idx != 1 || val != "/usr/bin" {
		t.Fatalf("findEnvVar(PATH) = (%d, %q), want (1, %q)", idx, val, "/usr/bin")
	}

	idx, val = findEnvVar(env, "doesnotexist")
	if idx != -1 || val != "" {
		t.Fatalf("findEnvVar(missing) = (%d, %q), want (-1, %q)", idx, val, "")
	}
}

func TestLooksLikePosixPathList(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"/usr/bin", false},            // single path, not a list
		{"/usr/bin:/bin", true},        // obvious list
		{"/c/Tools:/usr/bin", true},    // WSL-ish list
		{"C:/Windows/System32", false}, // single windows path with forward slashes
		{"C:/A:B", true},               // colon not at drive position acts like separator
		{"D:/one:/two", true},          // multiple colons implies separators
	}

	for _, tt := range tests {
		if got := looksLikePosixPathList(tt.in); got != tt.want {
			t.Fatalf("looksLikePosixPathList(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPosixToWindowsPath(t *testing.T) {
	got, ok := posixToWindowsPath("/c/Users/test")
	if !ok || got != `C:\Users\test` {
		t.Fatalf("posixToWindowsPath() = (%q, %v), want (%q, true)", got, ok, `C:\Users\test`)
	}

	_, ok = posixToWindowsPath("/usr/bin")
	if ok {
		t.Fatal("expected /usr/bin to not convert")
	}
}

func TestToShellPath(t *testing.T) {
	got := toShellPath(`C:\Tools\bin`, pathFlavorMSYS)
	if got != "/c/Tools/bin" {
		t.Fatalf("toShellPath(MSYS) = %q, want %q", got, "/c/Tools/bin")
	}

	got = toShellPath(`C:\Tools\bin`, pathFlavorWSL)
	if got != "/mnt/c/Tools/bin" {
		t.Fatalf("toShellPath(WSL) = %q, want %q", got, "/mnt/c/Tools/bin")
	}

	got = toShellPath(`/usr/bin`, pathFlavorMSYS)
	if got != "/usr/bin" {
		t.Fatalf("toShellPath(/usr/bin) = %q, want %q", got, "/usr/bin")
	}
}

func TestFixWindowsPathFromPosix(t *testing.T) {
	t.Run("no change when PATH already has semicolons", func(t *testing.T) {
		env := []string{"PATH=C:\\Tools;C:\\Bin"}
		got := fixWindowsPathFromPosix(append([]string{}, env...))
		if got[0] != env[0] {
			t.Fatalf("expected unchanged, got %q", got[0])
		}
	})

	t.Run("no change when PATH does not look like a posix list", func(t *testing.T) {
		env := []string{"PATH=C:/Windows/System32"}
		got := fixWindowsPathFromPosix(append([]string{}, env...))
		if got[0] != env[0] {
			t.Fatalf("expected unchanged, got %q", got[0])
		}
	})

	t.Run("converts /c/... style entries and joins with semicolons", func(t *testing.T) {
		env := []string{"PATH=/c/Tools:/usr/bin"}
		got := fixWindowsPathFromPosix(append([]string{}, env...))

		// /usr/bin is not convertible by design and stays as-is.
		want := `PATH=C:\Tools;/usr/bin`
		if got[0] != want {
			t.Fatalf("fixWindowsPathFromPosix() = %q, want %q", got[0], want)
		}
	})
}

func TestFixBashPath_NoRewriteWhenAlreadyPosixList(t *testing.T) {
	env := []string{"PATH=/c/Tools:/usr/bin"}
	got := fixBashPath(append([]string{}, env...), pathFlavorMSYS)
	if got[0] != env[0] {
		t.Fatalf("expected unchanged, got %q", got[0])
	}
}

func TestDetectShellFlavorPath(t *testing.T) {
	if got := detectShellFlavorPath(`C:\Windows\System32\bash.exe`); got != pathFlavorWSL {
		t.Fatalf("expected WSL flavor for system32 bash, got %v", got)
	}
	if got := detectShellFlavorPath(`C:\Program Files\Git\usr\bin\bash.exe`); got != pathFlavorMSYS {
		t.Fatalf("expected MSYS flavor for git bash, got %v", got)
	}
}

func TestFindGitShell(t *testing.T) {
	root := t.TempDir()

	// Pretend ProgramFiles points at our temp root.
	t.Setenv("ProgramFiles", root)
	t.Setenv("ProgramFiles(x86)", "")

	want := filepath.Join(root, "Git", "usr", "bin", "bash.exe")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(want, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := findGitShell("bash")
	if got != want {
		t.Fatalf("findGitShell(bash) = %q, want %q", got, want)
	}
}
