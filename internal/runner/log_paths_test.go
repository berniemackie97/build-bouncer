package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
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

func TestDefaultCheckLogsUseLinkedGitMetadata(t *testing.T) {
	base := t.TempDir()
	checkout := filepath.Join(base, "checkout")
	meta := filepath.Join(base, "meta")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(meta, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, ".git"), []byte("gitdir: ../meta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(meta, "build-bouncer", "logs")
	wantInsults := filepath.Join(meta, "build-bouncer", "insults_state.json")
	if got := resolveInsultStatePath(checkout); got != wantInsults {
		t.Fatalf("worktree insult history path: got=%q want=%q", got, wantInsults)
	}
	if err := saveInsultState(wantInsults, insultState{Version: 1, Recent: []string{"failed"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wantInsults); err != nil {
		t.Fatalf("insult history was not stored under Git metadata: %v", err)
	}
	if got := resolveDefaultLogDir(checkout); got != wantDir {
		t.Fatalf("worktree log path: got=%q want=%q", got, wantDir)
	}
	report, err := RunAllReport(checkout, &config.Config{Checks: []config.Check{{
		Name: "failed", Run: failCommand("worktree failure"),
	}}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.LogFiles["failed"] == "" ||
		filepath.Dir(report.LogFiles["failed"]) != wantDir {
		t.Fatalf("failed check log escaped Git metadata: %+v", report)
	}
	if _, err := os.Stat(report.LogFiles["failed"]); err != nil {
		t.Fatalf("failed check log was not persisted: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(checkout, ".buildbouncer", "logs")); !os.IsNotExist(err) {
		t.Fatalf("logs were created inside checkout: %v", err)
	}
}
