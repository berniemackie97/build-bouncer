//go:build !windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestInteractiveGitPushOverrideEnforcement(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script is required for a terminal-backed Git push")
	}
	sourceDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "build-bouncer")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = sourceDir
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	for _, tc := range []struct {
		name, response                string
		checks                        []config.Check
		failFast, allowed, wantPrompt bool
	}{
		{name: "accept completed failure", response: "yes\n", checks: []config.Check{{Name: "lint", Run: "exit 7"}}, allowed: true, wantPrompt: true},
		{name: "decline completed failure", response: "no\n", checks: []config.Check{{Name: "lint", Run: "exit 7"}}, wantPrompt: true},
		{name: "refuse missing tool", response: "yes\n", checks: []config.Check{{Name: "lint", Run: "echo never", Requires: config.StringList{"build_bouncer_missing_executable_987654"}}}},
		{name: "refuse timeout", response: "yes\n", checks: []config.Check{{Name: "slow", Run: "sleep 2", Timeout: 100 * time.Millisecond}}},
		{name: "refuse fail-fast cancellation", response: "yes\n", checks: []config.Check{{Name: "lint", Run: "exit 7"}, {Name: "after", Run: "echo later"}}, failFast: true},
		{name: "refuse unexecuted suite", response: "yes\n", checks: []config.Check{{Name: "other-platform", Run: "echo never", OS: config.StringList{"windows"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := withTempRepo(t)
			cfg := &config.Config{Checks: tc.checks, Runner: config.Runner{FailFast: tc.failFast}, Protection: config.Protection{Level: "moderate"}}
			remote := preparePolicyPushRepo(t, repo, binary, cfg)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			var args []string
			if runtime.GOOS == "darwin" {
				args = []string{"-q", "/dev/null", "git", "-C", repo, "push", "origin", "HEAD:refs/heads/guarded"}
			} else {
				commandText := "git -C " + strconv.Quote(repo) + " push origin HEAD:refs/heads/guarded"
				args = []string{"-q", "-e", "-c", commandText, "/dev/null"}
			}
			command := exec.CommandContext(ctx, "script", args...)
			command.Stdin = strings.NewReader(tc.response)
			command.Env = append(os.Environ(), "TERM=dumb")
			output, err := command.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("interactive push did not terminate: %v\n%s", ctx.Err(), output)
			}
			if tc.allowed && err != nil {
				t.Fatalf("accepted push failed: %v\n%s", err, output)
			}
			if got := strings.Contains(string(output), "Push anyway?"); got != tc.wantPrompt {
				t.Fatalf("interactive prompt present=%t want=%t; push error=%v\n%s", got, tc.wantPrompt, err, output)
			}
			if tc.allowed && !strings.Contains(string(output), "Override accepted") {
				t.Fatalf("accepted override not reported: %s", output)
			}
			assertPolicyRemoteRef(t, remote, tc.allowed)
		})
	}
}
