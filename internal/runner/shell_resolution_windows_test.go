//go:build windows

package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestWindowsGenericBashPrefersGitOverWSL(t *testing.T) {
	root := t.TempDir()
	system := filepath.Join(root, "Windows", "System32")
	if err := os.MkdirAll(system, 0o755); err != nil {
		t.Fatal(err)
	}
	wslBash := filepath.Join(system, "bash.exe")
	if err := os.WriteFile(wslBash, []byte("launcher"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", system)
	t.Setenv("ProgramFiles", root)
	t.Setenv("ProgramFiles(x86)", "")

	gitBash := filepath.Join(root, "Git", "usr", "bin", "bash.exe")
	if err := os.MkdirAll(filepath.Dir(gitBash), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gitBash, []byte("shell"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, spec := range []string{"bash", "bash.exe"} {
		executable, args, err := commandForShell(spec, "echo ok")
		if err != nil || executable != gitBash || len(args) != 2 || args[0] != "-lc" {
			t.Fatalf("commandForShell(%q): %q, %q, %v", spec, executable, args, err)
		}
	}

	check := config.Check{Name: "git-bash", Shell: "bash", Run: "echo ok"}
	if executable, err := ResolveCheckExecutable(check.Shell, check.Run); err != nil || executable != gitBash {
		t.Fatalf("runner readiness did not select Git Bash: %q, %v", executable, err)
	}
	if reason := SkipReason(check); reason != "" {
		t.Fatalf("installed Git Bash was misclassified as unavailable: %s", reason)
	}

	if err := os.Remove(gitBash); err != nil {
		t.Fatal(err)
	}
	if _, _, err := commandForShell("bash", "echo ok"); err == nil ||
		!strings.Contains(err.Error(), "WSL launcher") {
		t.Fatalf("bare bash must reject a WSL-only environment: %v", err)
	}
	check.AllowMissingTools = true
	report, err := RunAllReport(t.TempDir(), &config.Config{Checks: []config.Check{check}}, Options{})
	if err != nil || len(report.Failures) != 1 || len(report.Skipped) != 0 ||
		!strings.Contains(report.Unavailable[check.Name], "WSL launcher") {
		t.Fatalf("WSL-only implicit shell must fail, not skip: %+v, %v", report, err)
	}

	executable, args, err := commandForShell(wslBash, "echo ok")
	if err != nil || executable != wslBash || len(args) != 2 {
		t.Fatalf("explicit WSL path was not preserved: %q, %q, %v", executable, args, err)
	}
}

func TestWindowsGenericBashCanFindGitOutsidePATH(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramFiles", root)
	t.Setenv("ProgramFiles(x86)", "")
	t.Setenv("PATH", filepath.Join(root, "empty"))

	gitBash := filepath.Join(root, "Git", "bin", "bash.exe")
	if err := os.MkdirAll(filepath.Dir(gitBash), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gitBash, []byte("shell"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := preferWindowsShell("bash")
	if err != nil || got != gitBash {
		t.Fatalf("expected Git Bash location %q, got %q: %v", gitBash, got, err)
	}
}
