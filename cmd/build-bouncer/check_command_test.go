package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
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
