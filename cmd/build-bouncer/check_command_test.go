package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/runner"
)

func TestCheckRejectsForcePushBypass(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCheck([]string{"--force-push"}, cli.Context{Stdout: &stdout, Stderr: &stderr})
	if code != exitUsage {
		t.Fatalf("expected usage error, got %d", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("expected unknown flag diagnostic, got %q", stderr.String())
	}
}

func TestCIModeBlocksLaxFailures(t *testing.T) {
	root := withTempRepo(t)
	cfg := &config.Config{
		Checks:     []config.Check{{Name: "lint", Run: "exit 7"}},
		Protection: config.Protection{Level: "lax"},
	}
	if err := config.Save(filepath.Join(root, ".buildbouncer", "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if got := runCheck([]string{"--ci"}, cli.Context{Stdout: &stdout, Stderr: &stderr}); got != exitRunFailed {
		t.Fatalf("CI mode must block failures regardless of local policy: exit=%d stderr=%q", got, stderr.String())
	}
}

func TestCheckLaxRequiresExplicitFailurePermission(t *testing.T) {
	cases := []struct {
		name, level string
		allow       bool
		ci          bool
		want        int
	}{
		{name: "lint still blocks", level: "lax", want: exitRunFailed},
		{name: "explicit local exception", level: "lax", allow: true, want: exitOK},
		{name: "strict ignores exception", level: "strict", allow: true, want: exitRunFailed},
		{name: "moderate ignores exception", level: "moderate", allow: true, want: exitRunFailed},
		{name: "CI ignores exception", level: "lax", allow: true, ci: true, want: exitRunFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := withTempRepo(t)
			cfg := &config.Config{
				Checks:     []config.Check{{Name: "lint", Run: "exit 7", AllowFailure: tc.allow}},
				Protection: config.Protection{Level: tc.level},
			}
			if err := config.Save(filepath.Join(root, ".buildbouncer", "config.yaml"), cfg); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			args := []string{}
			if tc.ci {
				args = append(args, "--ci")
			}
			if got := runCheck(args, cli.Context{Stdout: &stdout, Stderr: &stderr}); got != tc.want {
				t.Fatalf("unexpected exit %d want %d; stdout=%q stderr=%q", got, tc.want, stdout.String(), stderr.String())
			}
			if tc.want == exitOK && !strings.Contains(stdout.String(), "explicitly allowed") {
				t.Fatalf("allowed local failure must be explained: %q", stdout.String())
			}
		})
	}
}

func TestCheckAllSkippedReturnsExecutionFailure(t *testing.T) {
	root := withTempRepo(t)
	unavailableOS := "windows"
	if runtime.GOOS == "windows" {
		unavailableOS = "linux"
	}
	cfg := &config.Config{Checks: []config.Check{{Name: "platform-only", Run: "echo never", OS: config.StringList{unavailableOS}}}}
	if err := config.Save(filepath.Join(root, ".buildbouncer", "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {"--ci"}, {"--hook"}} {
		var stdout, stderr bytes.Buffer
		got := runCheck(args, cli.Context{Stdout: &stdout, Stderr: &stderr})
		if got != exitRunFailed || !strings.Contains(stderr.String(), "no checks executed") ||
			strings.Contains(stdout.String(), "All checks passed") {
			t.Fatalf("all skipped accepted with %v: exit=%d stdout=%q stderr=%q", args, got, stdout.String(), stderr.String())
		}
	}
}

func TestCIStreamsRunnerOutputToCaller(t *testing.T) {
	root := withTempRepo(t)
	cfg := &config.Config{Checks: []config.Check{{Name: "visible-check", Run: "echo output-from-check"}}}
	if err := config.Save(filepath.Join(root, ".buildbouncer", "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runCheck([]string{"--ci"}, cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitOK {
		t.Fatalf("CI check failed: %d %q", code, stderr.String())
	}
	for _, expected := range []string{"==> visible-check", "output-from-check", "OK visible-check", "All checks passed"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("runner output bypassed CLI writer: missing %q from %q", expected, stdout.String())
		}
	}
}

func TestParallelHookUIWithAndWithoutBanter(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "without banter"
		if enabled {
			name = "with banter"
		}
		t.Run(name, func(t *testing.T) {
			repo := withTempRepo(t)
			checks := make([]config.Check, 24)
			for i := range checks {
				checks[i] = config.Check{Name: fmt.Sprintf("parallel-%02d", i), Run: "echo ok"}
			}
			cfg := &config.Config{
				Checks: checks,
				Runner: config.Runner{MaxParallel: 8},
				Banter: config.Banter{Enabled: &enabled, File: ".buildbouncer/assets/banter/test.json", Locale: "en"},
			}
			path := filepath.Join(repo, ".buildbouncer", "config.yaml")
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			packPath := filepath.Join(repo, ".buildbouncer", "assets", "banter", "test.json")
			if err := os.MkdirAll(filepath.Dir(packPath), 0o700); err != nil {
				t.Fatal(err)
			}
			pack := []byte(`{"version":1,"entries":[{"id":"intro","type":"intro","text":"hello"},{"id":"loading","type":"loading","text":"checking"},{"id":"success","type":"success","text":"done"}]}`)
			if err := os.WriteFile(packPath, pack, 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := runCheck([]string{"--hook"}, cli.Context{Stdout: &stdout, Stderr: &stderr})
			if code != exitOK {
				t.Fatalf("parallel hook run failed: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			statePath := filepath.Join(repo, ".git", "build-bouncer", "banter_state.json")
			content, err := os.ReadFile(statePath)
			if enabled {
				if err != nil || !bytes.Contains(content, []byte(`"loading"`)) {
					t.Fatalf("banter history lost under parallel progress: %q %v", content, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("banter disabled but history was persisted: %q %v", content, err)
			}
		})
	}
}

func TestPrintCanceledCheckEvidence(t *testing.T) {
	report := runner.Report{
		Canceled:     []string{"first", "second", "not-started"},
		LogFiles:     map[string]string{"first": "first.log", "second": "second.log"},
		LogTruncated: map[string]bool{"second": true},
	}
	var output bytes.Buffer
	printCanceledChecks(&output, report)
	for _, expected := range []string{"Canceled Checks", "first", "first.log", "second", "second.log", "truncated", "not-started"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q from canceled-check summary: %q", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "not-started.log") {
		t.Fatalf("log path was invented: %q", output.String())
	}
}
