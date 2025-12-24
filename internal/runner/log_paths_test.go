package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDefaultLogDir(t *testing.T) {
	t.Run("uses .git/build-bouncer/logs when .git directory exists", func(t *testing.T) {
		repoRoot := t.TempDir()

		gitDir := filepath.Join(repoRoot, ".git")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatalf("mkdir .git: %v", err)
		}

		got := resolveDefaultLogDir(repoRoot)
		want := filepath.Join(repoRoot, ".git", "build-bouncer", "logs")

		if got != want {
			t.Fatalf("resolveDefaultLogDir() = %q, want %q", got, want)
		}
	})

	t.Run("uses .buildbouncer/logs when .git does not exist", func(t *testing.T) {
		repoRoot := t.TempDir()

		got := resolveDefaultLogDir(repoRoot)
		want := filepath.Join(repoRoot, ".buildbouncer", "logs")

		if got != want {
			t.Fatalf("resolveDefaultLogDir() = %q, want %q", got, want)
		}
	})

	t.Run("uses .buildbouncer/logs when .git exists but is a file", func(t *testing.T) {
		repoRoot := t.TempDir()

		gitPath := filepath.Join(repoRoot, ".git")
		if err := os.WriteFile(gitPath, []byte("not a dir"), 0o644); err != nil {
			t.Fatalf("write .git file: %v", err)
		}

		got := resolveDefaultLogDir(repoRoot)
		want := filepath.Join(repoRoot, ".buildbouncer", "logs")

		if got != want {
			t.Fatalf("resolveDefaultLogDir() = %q, want %q", got, want)
		}
	})
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty becomes default", in: "", want: "check"},
		{name: "whitespace becomes default", in: "   \t\r\n", want: "check"},
		{name: "keeps safe characters", in: "hello-world_ok.1", want: "hello-world_ok.1"},
		{name: "trims then replaces spaces", in: "  Foo Bar  ", want: "Foo_Bar"},
		{name: "replaces path separators", in: `a/b\c`, want: "a_b_c"},
		{name: "replaces punctuation", in: "name:with:colons", want: "name_with_colons"},
		{name: "replaces emoji", in: "💥boom", want: "_boom"},
		{name: "replaces non-ascii", in: "汉字", want: "__"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitize(tt.in)
			if got != tt.want {
				t.Fatalf("sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	root := t.TempDir()

	existingFile := filepath.Join(root, "file.txt")
	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	existingDir := filepath.Join(root, "dir")
	if err := os.MkdirAll(existingDir, 0o755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}

	if !fileExists(existingFile) {
		t.Fatalf("expected fileExists(%q) to be true", existingFile)
	}
	if fileExists(existingDir) {
		t.Fatalf("expected fileExists(%q) to be false for directory", existingDir)
	}
	if fileExists(filepath.Join(root, "missing")) {
		t.Fatalf("expected fileExists(missing) to be false")
	}
}
