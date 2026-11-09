package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveGitDirWithRealLinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable is required")
	}
	base := t.TempDir()
	source := filepath.Join(base, "source")
	linked := filepath.Join(base, "linked")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("-C", source, "init")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("example"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("-C", source, "add", "README.md")
	runGit("-C", source, "-c", "user.name=BuildBouncer Test", "-c", "user.email=bb@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "initial")
	runGit("-C", source, "worktree", "add", "-b", "linked-checkout", linked)

	normal, ok := ResolveGitDir(source)
	if !ok || filepath.Clean(normal) != filepath.Join(source, ".git") {
		t.Fatalf("normal repository metadata: path=%q ok=%v", normal, ok)
	}
	got, ok := ResolveGitDir(linked)
	want := filepath.Clean(runGit("-C", linked, "rev-parse", "--absolute-git-dir"))
	if !ok || filepath.Clean(got) != want {
		t.Fatalf("linked checkout metadata: got=%q want=%q ok=%v", got, want, ok)
	}
	if got == filepath.Join(linked, ".git") {
		t.Fatal("linked checkout pointer was treated as a directory")
	}
}

func TestResolveGitDirPointerValidation(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "checkout")
	metadata := filepath.Join(base, "metadata")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(repo, ".git")
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "relative", value: "gitdir: ../metadata\n", want: metadata},
		{name: "absolute", value: "gitdir: "+metadata+"\n", want: metadata},
		{name: "mixed case", value: "GiTdIr: ../metadata\n", want: metadata},
		{name: "invalid marker", value: "not a git directory\n"},
		{name: "dangling", value: "gitdir: ../missing\n"},
		{name: "empty target", value: "gitdir:   \n"},
		{name: "file target", value: "gitdir: .git\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(marker, []byte(tc.value), 0o644); err != nil {
				t.Fatal(err)
			}
			got, ok := ResolveGitDir(repo)
			if ok != (tc.want != "") || (tc.want != "" && got != tc.want) {
				t.Fatalf("metadata for %q: got=%q ok=%v want=%q", tc.value, got, ok, tc.want)
			}
		})
	}
}
