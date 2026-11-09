package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestCheckStartFailureUsesFailedExecutionExitInAllModes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		level string
		allow bool
	}{
		{name: "normal strict", level: "strict"},
		{name: "lax with allowed ordinary failures", level: "lax", allow: true},
		{name: "moderate hook", level: "moderate", args: []string{"--hook"}},
		{name: "CI ignores failure exemption", level: "lax", allow: true, args: []string{"--ci"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := withTempRepo(t)
			cfg := &config.Config{
				Checks: []config.Check{
					{Name: "cannot-start", Run: "echo never", Cwd: "missing-workdir", AllowFailure: tc.allow},
					{Name: "never-started", Run: "echo never"},
				},
				Protection: config.Protection{Level: tc.level},
				Runner:     config.Runner{MaxParallel: 1, FailFast: true},
			}
			if err := config.Save(config.DefaultConfigPath(root), cfg); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code := runCheck(tc.args, cli.Context{Stdout: &stdout, Stderr: &stderr})
			if code != exitRunFailed {
				t.Fatalf("startup failure returned %d instead of check failure; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
			}
			for _, required := range []string{"check execution failed", "cannot-start", "missing-workdir", "never-started", "Failed Checks", "Canceled Checks"} {
				if !strings.Contains(stderr.String(), required) {
					t.Fatalf("missing %q from failed execution diagnostics: %q", required, stderr.String())
				}
			}
			if strings.Contains(stdout.String(), "All checks passed") ||
				strings.Contains(stdout.String(), "Override accepted") {
				t.Fatalf("failed check was presented as a passing push: %q", stdout.String())
			}
		})
	}
}

func TestCheckStartFailureShowsEarlierRetainedFailureLog(t *testing.T) {
	root := withTempRepo(t)
	cfg := &config.Config{Checks: []config.Check{
		{Name: "first-failed-check", Run: "exit 7"},
		{Name: "missing-directory", Run: "echo never", Cwd: "missing-workdir"},
	}}
	if err := config.Save(config.DefaultConfigPath(root), cfg); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runCheck(nil, cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitRunFailed {
		t.Fatalf("check failed to block: exit=%d stderr=%q", code, stderr.String())
	}
	for _, detail := range []string{"first-failed-check", "missing-directory", "missing-workdir", "Full log:", "build-bouncer-"} {
		if !strings.Contains(stderr.String(), detail) {
			t.Fatalf("missing saved failure evidence %q: %s", detail, stderr.String())
		}
	}
	if strings.Contains(stdout.String(), "All checks passed") {
		t.Fatalf("runtime error was presented as success: %q", stdout.String())
	}
}
