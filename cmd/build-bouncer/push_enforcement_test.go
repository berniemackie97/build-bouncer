package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestGitPushEnforcesConfiguredPolicy(t *testing.T) {
	sourceDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "build-bouncer")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = sourceDir
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual CLI: %v\n%s", err, output)
	}

	for _, tc := range []struct {
		name, level string
		allowFailure, requiredToolMissing, expectPush bool
	}{
		{name: "lax check name does not bypass", level: "lax"},
		{name: "lax explicit local exception", level: "lax", allowFailure: true, expectPush: true},
		{name: "lax missing mandatory tool", level: "lax", allowFailure: true, requiredToolMissing: true},
		{name: "strict rejects explicit local exception", level: "strict", allowFailure: true},
		{name: "moderate noninteractive rejects explicit local exception", level: "moderate", allowFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := withTempRepo(t)
			check := config.Check{Name: "lint", Run: "exit 7", AllowFailure: tc.allowFailure}
			if tc.requiredToolMissing {
				check.Requires = config.StringList{"build_bouncer_missing_executable_987654"}
			}
			cfg := &config.Config{
				Checks: []config.Check{check},
				Protection: config.Protection{Level: tc.level},
			}
			remote := preparePolicyPushRepo(t, repo, binary, cfg)
			output, err := policyGitPush(repo, "origin", "HEAD:refs/heads/guarded")
			if (err == nil) != tc.expectPush {
				t.Fatalf("push success=%t want %t: %v\n%s", err == nil, tc.expectPush, err, output)
			}
			if !tc.expectPush && !strings.Contains(output, "lint") {
				t.Fatalf("push did not demonstrate check enforcement: %s", output)
			}
			assertPolicyRemoteRef(t, remote, tc.expectPush)
		})
	}

	t.Run("force push still enforces and no-verify explicitly bypasses", func(t *testing.T) {
		repo := withTempRepo(t)
		cfg := &config.Config{Checks: []config.Check{{Name: "tests", Run: "exit 7"}}, Protection: config.Protection{Level: "strict"}}
		remote := preparePolicyPushRepo(t, repo, binary, cfg)
		for _, flags := range [][]string{{}, {"--force"}, {"--force-with-lease"}} {
			args := append(append([]string{}, flags...), "origin", "HEAD:refs/heads/guarded")
			output, err := policyGitPush(repo, args...)
			if err == nil || !strings.Contains(output, "tests") {
				t.Fatalf("push %v bypassed required checks: %v\n%s", flags, err, output)
			}
			assertPolicyRemoteRef(t, remote, false)
		}
		output, err := policyGitPush(repo, "--no-verify", "origin", "HEAD:refs/heads/guarded")
		if err != nil {
			t.Fatalf("explicit Git bypass did not succeed: %v\n%s", err, output)
		}
		assertPolicyRemoteRef(t, remote, true)
	})
}

func preparePolicyPushRepo(t *testing.T, repo, binary string, cfg *config.Config) string {
	t.Helper()
	if err := config.Save(filepath.Join(repo, ".buildbouncer", "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(t.TempDir(), "remote.git")
	for _, args := range [][]string{
		{"init", "-q", "--bare", remote},
		{"-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial"},
		{"-C", repo, "remote", "add", "origin", remote},
	} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	install := exec.Command(binary, "hook", "install")
	install.Dir = repo
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install actual CLI hook: %v\n%s", err, output)
	}
	return remote
}

func policyGitPush(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo, "push"}, args...)...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func assertPolicyRemoteRef(t *testing.T, remote string, present bool) {
	t.Helper()
	cmd := exec.Command("git", "--git-dir", remote, "show-ref", "--verify", "refs/heads/guarded")
	err := cmd.Run()
	if (err == nil) != present {
		t.Fatalf("remote guarded branch present=%t, want %t: %v", err == nil, present, err)
	}
}
