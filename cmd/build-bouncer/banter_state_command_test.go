package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestHookReportsBanterPersistenceFailureWithoutBlockingChecks(t *testing.T) {
	repo := withTempRepo(t)
	enabled := true
	cfg := &config.Config{
		Checks: []config.Check{{Name: "passing", Run: "echo ready"}},
		Banter: config.Banter{Enabled: &enabled, File: ".buildbouncer/assets/banter/test.json", Locale: "en"},
	}
	if err := config.Save(filepath.Join(repo, ".buildbouncer", "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}
	packPath := filepath.Join(repo, ".buildbouncer", "assets", "banter", "test.json")
	if err := os.MkdirAll(filepath.Dir(packPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packPath, []byte(`{"entries":[{"id":"intro","type":"intro","text":"hello"},{"id":"loading","type":"loading","text":"working"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(repo, ".git", "build-bouncer", "banter_state.json")
	lockPath := statePath + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(lockPath)

	var stdout, stderr bytes.Buffer
	code := runCheck([]string{"--hook"}, cli.Context{Stdout: &stdout, Stderr: &stderr})
	if code != exitOK || !strings.Contains(stderr.String(), "banter state:") ||
		!strings.Contains(stderr.String(), "banter_state.json.lock") {
		t.Fatalf("cosmetic persistence failure misreported: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state was written despite an active lock: %v", err)
	}
}
