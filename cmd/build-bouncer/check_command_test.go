package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
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
