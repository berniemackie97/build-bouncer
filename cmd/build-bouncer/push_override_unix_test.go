//go:build !windows

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
			stdin, input, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			defer input.Close()
			terminal := &pushTerminalOutput{ready: make(chan struct{})}
			command.Stdin = stdin
			command.Stdout = terminal
			command.Stderr = terminal
			command.Env = append(os.Environ(), "TERM=dumb")
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() { finished <- command.Wait() }()
			select {
			case <-terminal.ready:
				_, writeErr := io.WriteString(input, tc.response)
				err = <-finished
				if tc.allowed && writeErr != nil {
					t.Fatalf("send terminal response: %v", writeErr)
				}
			case err = <-finished:
			case <-ctx.Done():
				input.Close()
				err = <-finished
			}
			output := terminal.String()
			if ctx.Err() != nil {
				t.Fatalf("interactive push did not terminate: %v\n%s", ctx.Err(), output)
			}
			if tc.allowed && err != nil {
				t.Fatalf("accepted push failed: %v\n%s", err, output)
			}
			if got := strings.Contains(output, "Push anyway?"); got != tc.wantPrompt {
				t.Fatalf("interactive prompt present=%t want=%t; push error=%v\n%s", got, tc.wantPrompt, err, output)
			}
			if tc.allowed && !strings.Contains(output, "Override accepted") {
				t.Fatalf("accepted override not reported: %s", output)
			}
			assertPolicyRemoteRef(t, remote, tc.allowed)
		})
	}
}

type pushTerminalOutput struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	ready    chan struct{}
	notified bool
}

func (o *pushTerminalOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n, err := o.buffer.Write(p)
	if !o.notified {
		output := o.buffer.String()
		if strings.Contains(output, "Push anyway?") ||
			strings.Contains(output, "Push blocked: unavailable") ||
			strings.Contains(output, "no checks executed") {
			o.notified = true
			close(o.ready)
		}
	}
	return n, err
}

func (o *pushTerminalOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}
